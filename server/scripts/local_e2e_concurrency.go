package main

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jimeng-relay/server/internal/config"
	internalerrors "github.com/jimeng-relay/server/internal/errors"
	relayhandler "github.com/jimeng-relay/server/internal/handler/relay"
	"github.com/jimeng-relay/server/internal/middleware/observability"
	"github.com/jimeng-relay/server/internal/middleware/sigv4"
	"github.com/jimeng-relay/server/internal/relay/upstream"
	"github.com/jimeng-relay/server/internal/repository/sqlite"
	"github.com/jimeng-relay/server/internal/secretcrypto"
	apikeyservice "github.com/jimeng-relay/server/internal/service/apikey"
	auditservice "github.com/jimeng-relay/server/internal/service/audit"
	idempotencyservice "github.com/jimeng-relay/server/internal/service/idempotency"
	"github.com/jimeng-relay/server/internal/service/keymanager"
)

const (
	defaultRelayPort    = 18081
	defaultUpstreamPort = 18080

	defaultSubmitDelay      = 400 * time.Millisecond
	defaultResultReadyAfter = 400 * time.Millisecond

	xDateLayout = "20060102T150405Z"
)

type assertion struct {
	Name    string         `json:"name"`
	OK      bool           `json:"ok"`
	Details map[string]any `json:"details,omitempty"`
}

type artifact struct {
	OK          bool        `json:"ok"`
	Error       string      `json:"error,omitempty"`
	StartedAt   time.Time   `json:"started_at"`
	DurationMs  int64       `json:"duration_ms"`
	RelayURL    string      `json:"relay_url"`
	UpstreamURL string      `json:"upstream_url"`
	Assertions  []assertion `json:"assertions"`
}

func main() {
	if err := run(); err != nil {
		log.Printf("FAIL: %v", err)
		os.Exit(1)
	}
	log.Printf("PASS")
}

func run() (runErr error) {
	startedAt := time.Now().UTC()
	a := artifact{StartedAt: startedAt, Assertions: make([]assertion, 0)}
	var assertMu sync.Mutex
	addAssert := func(name string, ok bool, details map[string]any) {
		assertMu.Lock()
		defer assertMu.Unlock()
		a.Assertions = append(a.Assertions, assertion{Name: name, OK: ok, Details: details})
	}

	relayPort := envInt("E2E_RELAY_PORT", defaultRelayPort)
	upstreamPort := envInt("E2E_UPSTREAM_PORT", defaultUpstreamPort)
	cleanupPorts := envBool("E2E_CLEANUP_PORTS", true)
	keepTmp := envBool("E2E_KEEP_TMP", false)
	artifactPath := strings.TrimSpace(os.Getenv("E2E_ARTIFACT"))
	if artifactPath == "" {
		artifactPath = filepath.Join("scripts", "artifacts", "local_e2e_concurrency.json")
	}
	defer func() {
		a.OK = runErr == nil
		a.DurationMs = time.Since(startedAt).Milliseconds()
		if runErr != nil {
			a.Error = runErr.Error()
		}
		if err := writeArtifact(artifactPath, a); err != nil {
			if runErr == nil {
				runErr = err
			} else {
				log.Printf("artifact write failed: %v", err)
			}
			return
		}
		log.Printf("artifact: %s", artifactPath)
	}()

	submitDelay := envDuration("E2E_SUBMIT_DELAY", defaultSubmitDelay)
	resultReadyAfter := envDuration("E2E_RESULT_READY_AFTER", defaultResultReadyAfter)

	if cleanupPorts {
		if err := cleanupTCPPort(relayPort); err != nil {
			addAssert("cleanup_relay_port", false, map[string]any{"port": relayPort, "error": err.Error()})
			return err
		}
		addAssert("cleanup_relay_port", true, map[string]any{"port": relayPort})
		if err := cleanupTCPPort(upstreamPort); err != nil {
			addAssert("cleanup_upstream_port", false, map[string]any{"port": upstreamPort, "error": err.Error()})
			return err
		}
		addAssert("cleanup_upstream_port", true, map[string]any{"port": upstreamPort})
	}

	tmpDir, err := os.MkdirTemp("", "jimeng-relay-local-e2e-")
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}
	if !keepTmp {
		defer func() {
			if err := os.RemoveAll(tmpDir); err != nil {
				log.Printf("temp dir cleanup failed: %v", err)
			}
		}()
	}
	dbPath := filepath.Join(tmpDir, "jimeng-relay-e2e.db")

	rawEncKey := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, rawEncKey); err != nil {
		return fmt.Errorf("generate api key encryption key: %w", err)
	}
	secretCipher, err := secretcrypto.NewAESCipher(rawEncKey)
	if err != nil {
		return fmt.Errorf("init secret cipher: %w", err)
	}
	apiKeyEncryptionKeyB64 := base64.StdEncoding.EncodeToString(rawEncKey)

	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	repos, err := sqlite.Open(ctx, dbPath)
	if err != nil {
		return fmt.Errorf("open sqlite repos: %w", err)
	}
	defer func() {
		if err := repos.Close(); err != nil {
			log.Printf("sqlite repos close failed: %v", err)
		}
	}()

	auditSvc := auditservice.NewService(repos.DownstreamRequests, repos.UpstreamAttempts, repos.AuditEvents, auditservice.Config{})
	idempotencySvc := idempotencyservice.NewService(repos.IdempotencyRecords, idempotencyservice.Config{})
	km := keymanager.NewService(logger)

	region := "cn-north-1"
	upstreamURL := fmt.Sprintf("http://127.0.0.1:%d", upstreamPort)
	a.UpstreamURL = upstreamURL
	relayURL := fmt.Sprintf("http://127.0.0.1:%d", relayPort)
	a.RelayURL = relayURL

	fu := newFakeUpstream(submitDelay, resultReadyAfter)
	upstreamSrv, upstreamLn, err := startHTTPServer("fake-upstream", upstreamPort, fu.routes(), logger)
	if err != nil {
		return err
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if err := upstreamSrv.Shutdown(ctx); err != nil {
			log.Printf("fake-upstream shutdown failed: %v", err)
		}
		if err := upstreamLn.Close(); err != nil {
			log.Printf("fake-upstream listener close failed: %v", err)
		}
	}()

	uClientCfg := config.Config{
		Credentials:               config.Credentials{AccessKey: "dummy", SecretKey: "dummy"},
		Region:                    region,
		Host:                      upstreamURL,
		Timeout:                   5 * time.Second,
		ServerPort:                strconv.Itoa(relayPort),
		DatabaseType:              "sqlite",
		DatabaseURL:               dbPath,
		APIKeyEncryptionKey:       apiKeyEncryptionKeyB64,
		UpstreamMaxConcurrent:     1,
		UpstreamMaxQueue:          10,
		UpstreamSubmitMinInterval: 0,
	}
	upstreamClient, err := upstream.NewClient(uClientCfg, upstream.Options{KeyManager: km, MaxConcurrent: 1, MaxQueue: 10})
	if err != nil {
		return fmt.Errorf("init upstream client: %w", err)
	}

	relaySrv, relayLn, err := startHTTPServer("relay", relayPort, buildRelayMux(upstreamClient, repos, auditSvc, idempotencySvc, secretCipher, region, logger), logger)
	if err != nil {
		return err
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if err := relaySrv.Shutdown(ctx); err != nil {
			log.Printf("relay shutdown failed: %v", err)
		}
		if err := relayLn.Close(); err != nil {
			log.Printf("relay listener close failed: %v", err)
		}
	}()

	keySvc := apikeyservice.NewService(repos.APIKeys, apikeyservice.Config{SecretCipher: secretCipher})
	keyA, err := keySvc.Create(ctx, apikeyservice.CreateRequest{Description: "local-e2e-key-a"})
	if err != nil {
		return fmt.Errorf("create api key A: %w", err)
	}
	keyB, err := keySvc.Create(ctx, apikeyservice.CreateRequest{Description: "local-e2e-key-b"})
	if err != nil {
		return fmt.Errorf("create api key B: %w", err)
	}
	addAssert("create_api_keys", true, map[string]any{"key_a_id": keyA.ID, "key_b_id": keyB.ID})

	client := &http.Client{Timeout: 10 * time.Second}

	fu.Reset()
	submitBody := []byte(`{"prompt":"cat","req_key":"jimeng_t2i_v40"}`)
	type callResult struct {
		Status   int
		Headers  http.Header
		Body     []byte
		Duration time.Duration
		Err      error
	}
	submit1Ch := make(chan callResult, 1)
	go func() {
		status, hdr, body, dur, err := signedPOST(ctx, client, relayURL+"/v1/submit", submitBody, keyA.AccessKey, keyA.SecretKey, region, "cv")
		submit1Ch <- callResult{Status: status, Headers: hdr, Body: body, Duration: dur, Err: err}
	}()

	if err := fu.WaitForSubmitStarts(1, 3*time.Second); err != nil {
		addAssert("same_key_wait_upstream_start", false, map[string]any{"error": err.Error()})
		return fmt.Errorf("same-key: wait upstream submit start: %w", err)
	}

	status2, _, body2, dur2, err := signedPOST(ctx, client, relayURL+"/v1/submit", submitBody, keyA.AccessKey, keyA.SecretKey, region, "cv")
	if err != nil {
		addAssert("same_key_second_submit_call", false, map[string]any{"error": err.Error()})
		return fmt.Errorf("same-key: second submit call failed: %w", err)
	}
	code2 := extractErrorCode(body2)
	sameKeyRejected := status2 == http.StatusTooManyRequests && code2 == string(internalerrors.ErrRateLimited)
	addAssert("same_key_second_submit_rejected", sameKeyRejected, map[string]any{"status": status2, "error_code": code2, "duration_ms": dur2.Milliseconds()})
	if !sameKeyRejected {
		return fmt.Errorf("same-key: expected 429 RATE_LIMITED, got status=%d code=%q body=%s", status2, code2, string(body2))
	}

	submit1 := <-submit1Ch
	if submit1.Err != nil {
		addAssert("same_key_first_submit_call", false, map[string]any{"error": submit1.Err.Error()})
		return fmt.Errorf("same-key: first submit call failed: %w", submit1.Err)
	}
	taskID1, ok := extractTaskID(submit1.Body)
	addAssert("same_key_first_submit_ok", submit1.Status == http.StatusOK && ok, map[string]any{"status": submit1.Status, "task_id": taskID1, "duration_ms": submit1.Duration.Milliseconds()})
	if submit1.Status != http.StatusOK || !ok {
		return fmt.Errorf("same-key: expected 200 with task_id, got status=%d body=%s", submit1.Status, string(submit1.Body))
	}

	done, polls, lastBody, err := pollGetResult(ctx, client, relayURL, taskID1, keyA.AccessKey, keyA.SecretKey, region, "cv")
	addAssert("same_key_get_result_done", done, map[string]any{"task_id": taskID1, "polls": polls})
	if err != nil {
		return fmt.Errorf("same-key: get-result polling failed: %w body=%s", err, string(lastBody))
	}
	if !done {
		return fmt.Errorf("same-key: get-result never reached done")
	}
	addAssert("same_key_upstream_submit_calls", fu.SubmitCalls() == 1, map[string]any{"submit_calls": fu.SubmitCalls()})

	fu.Reset()
	submitACh := make(chan callResult, 1)
	submitBCh := make(chan callResult, 1)
	go func() {
		status, hdr, body, dur, err := signedPOST(ctx, client, relayURL+"/v1/submit", []byte(`{"prompt":"a","req_key":"jimeng_t2i_v40"}`), keyA.AccessKey, keyA.SecretKey, region, "cv")
		submitACh <- callResult{Status: status, Headers: hdr, Body: body, Duration: dur, Err: err}
	}()
	go func() {
		status, hdr, body, dur, err := signedPOST(ctx, client, relayURL+"/v1/submit", []byte(`{"prompt":"b","req_key":"jimeng_t2i_v40"}`), keyB.AccessKey, keyB.SecretKey, region, "cv")
		submitBCh <- callResult{Status: status, Headers: hdr, Body: body, Duration: dur, Err: err}
	}()

	resA := <-submitACh
	resB := <-submitBCh
	if resA.Err != nil || resB.Err != nil {
		addAssert("different_key_submit_calls", false, map[string]any{"err_a": errString(resA.Err), "err_b": errString(resB.Err)})
		return fmt.Errorf("different-key: submit errors a=%v b=%v", resA.Err, resB.Err)
	}
	taskA, okA := extractTaskID(resA.Body)
	taskB, okB := extractTaskID(resB.Body)
	bothOK := resA.Status == http.StatusOK && resB.Status == http.StatusOK && okA && okB
	addAssert("different_key_both_submit_ok", bothOK, map[string]any{"status_a": resA.Status, "status_b": resB.Status, "task_a": taskA, "task_b": taskB, "dur_a_ms": resA.Duration.Milliseconds(), "dur_b_ms": resB.Duration.Milliseconds()})
	if !bothOK {
		return fmt.Errorf("different-key: expected both 200 with task_id; got a=%d b=%d", resA.Status, resB.Status)
	}

	maxInFlight := fu.MaxInFlight()
	addAssert("different_key_upstream_max_inflight_is_one", maxInFlight == 1, map[string]any{"max_inflight": maxInFlight})
	if maxInFlight != 1 {
		return fmt.Errorf("different-key: expected upstream max inflight 1, got %d", maxInFlight)
	}
	addAssert("different_key_upstream_submit_calls", fu.SubmitCalls() == 2, map[string]any{"submit_calls": fu.SubmitCalls()})
	if fu.SubmitCalls() != 2 {
		return fmt.Errorf("different-key: expected 2 upstream submit calls, got %d", fu.SubmitCalls())
	}
	if submitDelay > 0 {
		starts := fu.SubmitStartTimes()
		var deltaMs int64
		if len(starts) == 2 {
			deltaMs = starts[1].Sub(starts[0]).Milliseconds()
		}
		serialized := len(starts) == 2 && starts[1].Sub(starts[0]) >= submitDelay/2
		addAssert("different_key_upstream_submit_serialized", serialized, map[string]any{"submit_delay_ms": submitDelay.Milliseconds(), "start_delta_ms": deltaMs})
		if len(starts) != 2 {
			return fmt.Errorf("different-key: expected 2 upstream submit starts, got %d", len(starts))
		}
		if !serialized {
			return fmt.Errorf("different-key: upstream submits not serialized as expected")
		}
	}

	grACh := make(chan error, 1)
	grBCh := make(chan error, 1)
	go func() {
		done, polls, _, err := pollGetResult(ctx, client, relayURL, taskA, keyA.AccessKey, keyA.SecretKey, region, "cv")
		if err != nil {
			grACh <- err
			return
		}
		if !done {
			grACh <- errors.New("task A never done")
			return
		}
		addAssert("different_key_get_result_done_a", true, map[string]any{"task_id": taskA, "polls": polls})
		grACh <- nil
	}()
	go func() {
		done, polls, _, err := pollGetResult(ctx, client, relayURL, taskB, keyB.AccessKey, keyB.SecretKey, region, "cv")
		if err != nil {
			grBCh <- err
			return
		}
		if !done {
			grBCh <- errors.New("task B never done")
			return
		}
		addAssert("different_key_get_result_done_b", true, map[string]any{"task_id": taskB, "polls": polls})
		grBCh <- nil
	}()

	if err := <-grACh; err != nil {
		addAssert("different_key_get_result_a", false, map[string]any{"error": err.Error()})
		return fmt.Errorf("different-key: get-result A failed: %w", err)
	}
	if err := <-grBCh; err != nil {
		addAssert("different_key_get_result_b", false, map[string]any{"error": err.Error()})
		return fmt.Errorf("different-key: get-result B failed: %w", err)
	}

	return nil
}

func buildRelayMux(upstreamClient *upstream.Client, repos *sqlite.Repositories, auditSvc *auditservice.Service, idempotencySvc *idempotencyservice.Service, cipher secretcrypto.Cipher, region string, logger *slog.Logger) http.Handler {
	authn := sigv4.New(repos.APIKeys, sigv4.Config{SecretCipher: cipher, ExpectedRegion: region, ExpectedService: "cv"})
	obs := observability.Middleware(logger)

	app := http.NewServeMux()
	submitRoutes := relayhandler.NewSubmitHandler(upstreamClient, auditSvc, idempotencySvc, repos.IdempotencyRecords, logger).Routes()
	getResultRoutes := relayhandler.NewGetResultHandler(upstreamClient, auditSvc, logger).Routes()
	app.Handle("/v1/submit", submitRoutes)
	app.Handle("/v1/get-result", getResultRoutes)
	app.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		action := r.URL.Query().Get("Action")
		switch action {
		case "CVSync2AsyncSubmitTask":
			submitRoutes.ServeHTTP(w, r)
		case "CVSync2AsyncGetResult":
			getResultRoutes.ServeHTTP(w, r)
		default:
			http.NotFound(w, r)
		}
	})

	mux := http.NewServeMux()
	mux.Handle("/", obs(authn(app)))
	return mux
}

func startHTTPServer(name string, port int, handler http.Handler, logger *slog.Logger) (*http.Server, net.Listener, error) {
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return nil, nil, fmt.Errorf("listen %s on port %d: %w", name, port, err)
	}
	srv := &http.Server{Handler: handler}
	go func() {
		if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			if logger != nil {
				logger.Error("server error", "name", name, "error", err.Error())
			}
		}
	}()
	return srv, ln, nil
}

type fakeUpstream struct {
	submitDelay      time.Duration
	resultReadyAfter time.Duration

	mu               sync.Mutex
	tasks            map[string]time.Time
	submitStartTimes []time.Time

	submitStarts int64
	submitCalls  int64
	getCalls     int64

	inFlight    int32
	maxInFlight int32

	submitStartCh chan struct{}
}

func newFakeUpstream(submitDelay, resultReadyAfter time.Duration) *fakeUpstream {
	return &fakeUpstream{
		submitDelay:      submitDelay,
		resultReadyAfter: resultReadyAfter,
		tasks:            make(map[string]time.Time),
		submitStartCh:    make(chan struct{}, 100),
	}
}

func (f *fakeUpstream) Reset() {
	f.mu.Lock()
	f.tasks = make(map[string]time.Time)
	f.submitStartTimes = nil
	atomic.StoreInt64(&f.submitStarts, 0)
	atomic.StoreInt64(&f.submitCalls, 0)
	atomic.StoreInt64(&f.getCalls, 0)
	atomic.StoreInt32(&f.inFlight, 0)
	atomic.StoreInt32(&f.maxInFlight, 0)
	for {
		select {
		case <-f.submitStartCh:
			continue
		default:
			f.mu.Unlock()
			return
		}
	}
}

func (f *fakeUpstream) SubmitCalls() int64 { return atomic.LoadInt64(&f.submitCalls) }
func (f *fakeUpstream) MaxInFlight() int32 { return atomic.LoadInt32(&f.maxInFlight) }

func (f *fakeUpstream) SubmitStartTimes() []time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.submitStartTimes) == 0 {
		return nil
	}
	out := make([]time.Time, len(f.submitStartTimes))
	copy(out, f.submitStartTimes)
	return out
}

func (f *fakeUpstream) WaitForSubmitStarts(n int64, timeout time.Duration) error {
	if n <= 0 {
		return nil
	}
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	for {
		if atomic.LoadInt64(&f.submitStarts) >= n {
			return nil
		}
		select {
		case <-f.submitStartCh:
			continue
		case <-deadline.C:
			return fmt.Errorf("timeout waiting submit starts: want=%d got=%d", n, atomic.LoadInt64(&f.submitStarts))
		}
	}
}

func (f *fakeUpstream) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", f.handle)
	return mux
}

func (f *fakeUpstream) handle(w http.ResponseWriter, r *http.Request) {
	action := strings.TrimSpace(r.URL.Query().Get("Action"))
	if r.Method != http.MethodPost || r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "read body", http.StatusInternalServerError)
		return
	}

	switch action {
	case "CVSync2AsyncSubmitTask":
		f.handleSubmit(w)
	case "CVSync2AsyncGetResult":
		f.handleGetResult(w, body)
	default:
		http.NotFound(w, r)
	}
}

func (f *fakeUpstream) handleSubmit(w http.ResponseWriter) {
	f.mu.Lock()
	f.submitStartTimes = append(f.submitStartTimes, time.Now().UTC())
	f.mu.Unlock()

	atomic.AddInt64(&f.submitCalls, 1)
	atomic.AddInt64(&f.submitStarts, 1)
	select {
	case f.submitStartCh <- struct{}{}:
	default:
	}

	in := atomic.AddInt32(&f.inFlight, 1)
	defer atomic.AddInt32(&f.inFlight, -1)
	for {
		max := atomic.LoadInt32(&f.maxInFlight)
		if in <= max {
			break
		}
		if atomic.CompareAndSwapInt32(&f.maxInFlight, max, in) {
			break
		}
	}

	if f.submitDelay > 0 {
		time.Sleep(f.submitDelay)
	}

	taskID := "task_" + randomHex(8)
	readyAt := time.Now().UTC().Add(f.resultReadyAfter)
	f.mu.Lock()
	f.tasks[taskID] = readyAt
	f.mu.Unlock()

	writeJSON(w, http.StatusOK, map[string]any{
		"code":    10000,
		"message": "ok",
		"data": map[string]any{
			"task_id": taskID,
		},
	})
}

func (f *fakeUpstream) handleGetResult(w http.ResponseWriter, body []byte) {
	atomic.AddInt64(&f.getCalls, 1)
	var payload struct {
		TaskID string `json:"task_id"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		payload.TaskID = ""
	}
	taskID := strings.TrimSpace(payload.TaskID)
	if taskID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"code": 40011, "message": "invalid task_id"})
		return
	}
	f.mu.Lock()
	readyAt, ok := f.tasks[taskID]
	f.mu.Unlock()
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]any{"code": 40011, "message": "invalid task_id"})
		return
	}
	status := "running"
	if !time.Now().UTC().Before(readyAt) {
		status = "done"
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"code":    10000,
		"message": "ok",
		"data": map[string]any{
			"status": status,
		},
	})
}

func pollGetResult(ctx context.Context, hc *http.Client, relayURL, taskID, accessKey, secretKey, region, service string) (done bool, polls int, lastBody []byte, err error) {
	deadline := time.Now().Add(10 * time.Second)
	for {
		if time.Now().After(deadline) {
			return false, polls, lastBody, fmt.Errorf("timeout")
		}
		body := []byte(fmt.Sprintf(`{"task_id":%q}`, taskID))
		status, _, respBody, _, reqErr := signedPOST(ctx, hc, relayURL+"/v1/get-result", body, accessKey, secretKey, region, service)
		polls++
		lastBody = respBody
		if reqErr != nil {
			return false, polls, lastBody, reqErr
		}
		if status != http.StatusOK {
			return false, polls, lastBody, fmt.Errorf("unexpected status %d", status)
		}
		st := extractDataStatus(respBody)
		if st == "done" {
			return true, polls, lastBody, nil
		}
		time.Sleep(150 * time.Millisecond)
	}
}

func signedPOST(ctx context.Context, hc *http.Client, endpoint string, body []byte, accessKey, secretKey, region, service string) (status int, headers http.Header, respBody []byte, dur time.Duration, err error) {
	if hc == nil {
		hc = http.DefaultClient
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return 0, nil, nil, 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Request-Id", "req_"+randomHex(8))
	if err := signAWS4(req, body, accessKey, secretKey, region, service, time.Now().UTC()); err != nil {
		return 0, nil, nil, 0, err
	}

	start := time.Now()
	resp, err := hc.Do(req)
	dur = time.Since(start)
	if err != nil {
		return 0, nil, nil, dur, err
	}
	b, readErr := io.ReadAll(resp.Body)
	closeErr := resp.Body.Close()
	if readErr != nil {
		return resp.StatusCode, resp.Header.Clone(), nil, dur, readErr
	}
	if closeErr != nil {
		return resp.StatusCode, resp.Header.Clone(), b, dur, closeErr
	}
	return resp.StatusCode, resp.Header.Clone(), b, dur, nil
}

func signAWS4(req *http.Request, body []byte, accessKey, secretKey, region, service string, now time.Time) error {
	if req == nil {
		return errors.New("nil request")
	}
	accessKey = strings.TrimSpace(accessKey)
	secretKey = strings.TrimSpace(secretKey)
	if accessKey == "" || secretKey == "" {
		return errors.New("missing access_key/secret_key")
	}
	region = strings.ToLower(strings.TrimSpace(region))
	service = strings.ToLower(strings.TrimSpace(service))
	if region == "" || service == "" {
		return errors.New("missing region/service")
	}
	if strings.TrimSpace(req.Host) == "" {
		if req.URL != nil {
			req.Host = req.URL.Host
		}
	}
	if strings.TrimSpace(req.Host) == "" {
		return errors.New("missing host")
	}

	xDate := now.UTC().Format(xDateLayout)
	dateScope := now.UTC().Format("20060102")
	payloadHash := sha256Hex(body)
	req.Header.Set("X-Date", xDate)
	req.Header.Set("X-Content-Sha256", payloadHash)

	signedHeaders := []string{"host", "x-content-sha256", "x-date"}
	sort.Strings(signedHeaders)
	canonicalRequest, err := buildCanonicalRequest(req, signedHeaders, payloadHash)
	if err != nil {
		return err
	}
	scopeSuffix := "aws4_request"
	scope := strings.Join([]string{dateScope, region, service, scopeSuffix}, "/")
	algorithm := "AWS4-HMAC-SHA256"
	stringToSign := strings.Join([]string{algorithm, xDate, scope, sha256Hex([]byte(canonicalRequest))}, "\n")
	signingKey := deriveSigningKey(secretKey, dateScope, region, service, scopeSuffix)
	signature := hex.EncodeToString(hmacSHA256(signingKey, stringToSign))
	authorization := algorithm + " Credential=" + accessKey + "/" + scope + ", SignedHeaders=" + strings.Join(signedHeaders, ";") + ", Signature=" + signature
	req.Header.Set("Authorization", authorization)
	return nil
}

func buildCanonicalRequest(r *http.Request, signedHeaders []string, payloadHash string) (string, error) {
	headers := append([]string(nil), signedHeaders...)
	sort.Strings(headers)
	canonHeaders := strings.Builder{}
	for _, h := range headers {
		h = strings.ToLower(strings.TrimSpace(h))
		if h == "" {
			return "", errors.New("empty signed header")
		}
		v := canonicalHeaderValue(r, h)
		if v == "" {
			return "", fmt.Errorf("missing signed header: %s", h)
		}
		canonHeaders.WriteString(h)
		canonHeaders.WriteByte(':')
		canonHeaders.WriteString(v)
		canonHeaders.WriteByte('\n')
	}
	return strings.Join([]string{
		r.Method,
		canonicalURI(r.URL.Path),
		canonicalQueryString(r.URL.Query()),
		canonHeaders.String(),
		strings.Join(headers, ";"),
		payloadHash,
	}, "\n"), nil
}

func canonicalHeaderValue(r *http.Request, name string) string {
	if strings.EqualFold(name, "host") {
		return strings.TrimSpace(strings.ToLower(r.Host))
	}
	vals := r.Header.Values(name)
	if len(vals) == 0 {
		return ""
	}
	for i := range vals {
		vals[i] = strings.Join(strings.Fields(vals[i]), " ")
	}
	return strings.TrimSpace(strings.Join(vals, ","))
}

func canonicalURI(path string) string {
	if path == "" {
		return "/"
	}
	parts := strings.Split(path, "/")
	for i := range parts {
		parts[i] = awsEscape(parts[i])
	}
	uri := strings.Join(parts, "/")
	if !strings.HasPrefix(uri, "/") {
		uri = "/" + uri
	}
	return uri
}

func canonicalQueryString(values url.Values) string {
	if len(values) == 0 {
		return ""
	}
	keys := make([]string, 0, len(values))
	for k := range values {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	pairs := make([]string, 0)
	for _, k := range keys {
		vals := append([]string(nil), values[k]...)
		sort.Strings(vals)
		ek := awsEscape(k)
		for _, v := range vals {
			pairs = append(pairs, ek+"="+awsEscape(v))
		}
	}
	return strings.Join(pairs, "&")
}

func awsEscape(s string) string {
	e := url.QueryEscape(s)
	e = strings.ReplaceAll(e, "+", "%20")
	e = strings.ReplaceAll(e, "*", "%2A")
	e = strings.ReplaceAll(e, "%7E", "~")
	return e
}

func deriveSigningKey(secret, date, region, service, suffix string) []byte {
	kDate := hmacSHA256([]byte("AWS4"+secret), date)
	kRegion := hmacSHA256(kDate, region)
	kService := hmacSHA256(kRegion, service)
	return hmacSHA256(kService, suffix)
}

func hmacSHA256(key []byte, message string) []byte {
	h := hmac.New(sha256.New, key)
	if _, err := h.Write([]byte(message)); err != nil {
		panic(err)
	}
	return h.Sum(nil)
}

func sha256Hex(v []byte) string {
	s := sha256.Sum256(v)
	return hex.EncodeToString(s[:])
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("writeJSON encode failed: %v", err)
	}
}

func randomHex(n int) string {
	if n <= 0 {
		n = 8
	}
	b := make([]byte, n)
	if _, err := io.ReadFull(rand.Reader, b); err != nil {
		return hex.EncodeToString([]byte(time.Now().UTC().Format(time.RFC3339Nano)))
	}
	return hex.EncodeToString(b)
}

func extractTaskID(body []byte) (string, bool) {
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return "", false
	}
	data, ok := payload["data"].(map[string]any)
	if !ok {
		return "", false
	}
	taskIDVal, ok := data["task_id"]
	if !ok {
		return "", false
	}
	taskID, ok := taskIDVal.(string)
	if !ok {
		return "", false
	}
	taskID = strings.TrimSpace(taskID)
	return taskID, taskID != ""
}

func extractDataStatus(body []byte) string {
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return ""
	}
	data, ok := payload["data"].(map[string]any)
	if !ok {
		return ""
	}
	stVal, ok := data["status"]
	if !ok {
		return ""
	}
	st, ok := stVal.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(st)
}

func extractErrorCode(body []byte) string {
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return ""
	}
	errObj, ok := payload["error"].(map[string]any)
	if !ok {
		return ""
	}
	codeVal, ok := errObj["code"]
	if !ok {
		return ""
	}
	code, ok := codeVal.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(code)
}

func writeArtifact(path string, a artifact) error {
	if strings.TrimSpace(path) == "" {
		return errors.New("empty artifact path")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("mkdir artifact dir: %w", err)
	}
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create artifact: %w", err)
	}
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err := enc.Encode(a); err != nil {
		if closeErr := f.Close(); closeErr != nil {
			log.Printf("artifact file close failed after encode error: %v", closeErr)
		}
		return fmt.Errorf("write artifact: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("close artifact: %w", err)
	}
	return nil
}

func envInt(key string, def int) int {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}

func envBool(key string, def bool) bool {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return def
	}
	return b
}

func envDuration(key string, def time.Duration) time.Duration {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return def
	}
	return d
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func cleanupTCPPort(port int) error {
	if port <= 0 {
		return nil
	}
	if runtime.GOOS == "windows" {
		return nil
	}
	if _, err := exec.LookPath("lsof"); err != nil {
		return nil
	}
	forceKill := envBool("E2E_FORCE_KILL", false)
	out, err := exec.Command("lsof", "-nP", "-iTCP:"+strconv.Itoa(port), "-sTCP:LISTEN", "-Fpct").Output()
	if err != nil {
		return nil
	}
	procs := parseLsofProcs(out)
	for _, proc := range procs {
		if proc.PID <= 0 {
			continue
		}
		if !forceKill && !allowKillCommand(proc.Command) {
			return fmt.Errorf("port %d is in use by %s (pid %d); set E2E_FORCE_KILL=true or change E2E_RELAY_PORT/E2E_UPSTREAM_PORT", port, proc.Command, proc.PID)
		}
	}
	for _, proc := range procs {
		if proc.PID <= 0 {
			continue
		}
		p, err := os.FindProcess(proc.PID)
		if err != nil {
			continue
		}
		if err := p.Signal(os.Interrupt); err != nil {
			log.Printf("failed to signal interrupt pid %d: %v", proc.PID, err)
		}
		if err := p.Signal(os.Kill); err != nil {
			log.Printf("failed to signal kill pid %d: %v", proc.PID, err)
		}
	}
	return nil
}

type lsofProc struct {
	PID     int
	Command string
}

func parseLsofProcs(out []byte) []lsofProc {
	lines := strings.Split(string(out), "\n")
	var procs []lsofProc
	cur := lsofProc{}
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if len(line) < 2 {
			continue
		}
		switch line[0] {
		case 'p':
			if cur.PID > 0 {
				procs = append(procs, cur)
			}
			pid, err := strconv.Atoi(strings.TrimSpace(line[1:]))
			if err != nil {
				cur = lsofProc{}
				continue
			}
			cur = lsofProc{PID: pid}
		case 'c':
			cur.Command = strings.TrimSpace(line[1:])
		}
	}
	if cur.PID > 0 {
		procs = append(procs, cur)
	}
	return procs
}

func allowKillCommand(cmd string) bool {
	cmd = strings.ToLower(strings.TrimSpace(cmd))
	if cmd == "" {
		return false
	}
	if strings.Contains(cmd, "jimeng") {
		return true
	}
	if strings.Contains(cmd, "local_e2e") {
		return true
	}
	return false
}
