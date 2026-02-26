package upstream_test

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jimeng-relay/server/internal/config"
	internalerrors "github.com/jimeng-relay/server/internal/errors"
	"github.com/jimeng-relay/server/internal/relay/upstream"
	"github.com/jimeng-relay/server/internal/service/keymanager"
	"github.com/volcengine/volc-sdk-golang/service/visual"
)

func TestClient_Submit_SignsAndCalls_AndDoesNotForwardDownstreamAuthorization(t *testing.T) {
	now := time.Date(2026, 2, 24, 10, 0, 0, 0, time.UTC)

	ak := "ak_test"
	sk := "sk_test"
	region := "cn-north-1"
	service := "cv"

	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("Action") != "CVSync2AsyncSubmitTask" {
			w.WriteHeader(http.StatusBadRequest)
			if _, err := w.Write([]byte("bad action")); err != nil {
				return
			}
			return
		}
		if r.URL.Query().Get("Version") != "2022-08-31" {
			w.WriteHeader(http.StatusBadRequest)
			if _, err := w.Write([]byte("bad version")); err != nil {
				return
			}
			return
		}
		gotAuth = strings.TrimSpace(r.Header.Get("Authorization"))
		if gotAuth == "Downstream abc" {
			w.WriteHeader(http.StatusBadRequest)
			if _, err := w.Write([]byte("downstream authorization forwarded")); err != nil {
				return
			}
			return
		}
		if !strings.HasPrefix(gotAuth, "HMAC-SHA256 ") {
			w.WriteHeader(http.StatusBadRequest)
			if _, err := w.Write([]byte("missing sigv4 authorization")); err != nil {
				return
			}
			return
		}
		if err := verifySigV4Request(r, ak, sk, region, service); err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			if encErr := json.NewEncoder(w).Encode(map[string]any{"error": err.Error()}); encErr != nil {
				return
			}
			return
		}
		w.Header().Set("X-Upstream", "1")
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write([]byte(`{"ok":true}`)); err != nil {
			return
		}
	}))
	t.Cleanup(srv.Close)

	c, err := upstream.NewClient(config.Config{
		Credentials: config.Credentials{AccessKey: ak, SecretKey: sk},
		Region:      region,
		Host:        srv.URL,
		Timeout:     2 * time.Second,
	}, upstream.Options{Now: func() time.Time { return now }})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	body := []byte(`{"prompt":"cat"}`)
	resp, callErr := c.Submit(context.Background(), body, http.Header{"Authorization": []string{"Downstream abc"}})
	if callErr != nil {
		t.Fatalf("Submit unexpected error: %v", callErr)
	}
	if resp == nil {
		t.Fatalf("expected resp")
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", resp.StatusCode, string(resp.Body))
	}
	if got := resp.Header.Get("X-Upstream"); got != "1" {
		t.Fatalf("expected upstream header, got %q", got)
	}
	if gotAuth == "" {
		t.Fatalf("expected upstream authorization header")
	}
}

func TestClient_Submit_UpstreamRejectsSignature_ReturnsErrUpstreamFailed_WithResponse(t *testing.T) {
	now := time.Date(2026, 2, 24, 10, 0, 0, 0, time.UTC)

	ak := "ak_test"
	region := "cn-north-1"
	service := "cv"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := verifySigV4Request(r, ak, "sk_expected", region, service); err != nil {
			w.WriteHeader(http.StatusUnauthorized)
			if _, writeErr := w.Write([]byte("bad signature")); writeErr != nil {
				return
			}
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	c, err := upstream.NewClient(config.Config{
		Credentials: config.Credentials{AccessKey: ak, SecretKey: "sk_wrong"},
		Region:      region,
		Host:        srv.URL,
		Timeout:     2 * time.Second,
	}, upstream.Options{Now: func() time.Time { return now }})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	resp, callErr := c.Submit(context.Background(), []byte(`{"prompt":"cat"}`), nil)
	if callErr == nil {
		t.Fatalf("expected error")
	}
	if internalerrors.GetCode(callErr) != internalerrors.ErrUpstreamFailed {
		t.Fatalf("unexpected error code: %s err=%v", internalerrors.GetCode(callErr), callErr)
	}
	if resp == nil {
		t.Fatalf("expected response even on upstream failure")
	}
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unexpected status: %d body=%s", resp.StatusCode, string(resp.Body))
	}
}

func TestClient_GetResult_SignsAndCalls(t *testing.T) {
	now := time.Date(2026, 2, 24, 10, 0, 0, 0, time.UTC)

	ak := "ak_test"
	sk := "sk_test"
	region := "cn-north-1"
	service := "cv"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("Action") != "CVSync2AsyncGetResult" {
			w.WriteHeader(http.StatusBadRequest)
			if _, err := w.Write([]byte("bad action")); err != nil {
				return
			}
			return
		}
		if r.URL.Query().Get("Version") != "2022-08-31" {
			w.WriteHeader(http.StatusBadRequest)
			if _, err := w.Write([]byte("bad version")); err != nil {
				return
			}
			return
		}
		if err := verifySigV4Request(r, ak, sk, region, service); err != nil {
			w.WriteHeader(http.StatusUnauthorized)
			if _, writeErr := w.Write([]byte("bad signature")); writeErr != nil {
				return
			}
			return
		}
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write([]byte(`{"status":"done"}`)); err != nil {
			return
		}
	}))
	t.Cleanup(srv.Close)

	c, err := upstream.NewClient(config.Config{
		Credentials: config.Credentials{AccessKey: ak, SecretKey: sk},
		Region:      region,
		Host:        srv.URL,
		Timeout:     2 * time.Second,
	}, upstream.Options{Now: func() time.Time { return now }})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	resp, callErr := c.GetResult(context.Background(), []byte(`{"task_id":"t1"}`), nil)
	if callErr != nil {
		t.Fatalf("GetResult unexpected error: %v", callErr)
	}
	if resp == nil {
		t.Fatalf("expected resp")
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", resp.StatusCode, string(resp.Body))
	}
}

func TestNewClient_MissingRequiredConfig_ReturnsErrValidationFailed(t *testing.T) {
	_, err := upstream.NewClient(config.Config{}, upstream.Options{})
	if err == nil {
		t.Fatalf("expected error")
	}
	if internalerrors.GetCode(err) != internalerrors.ErrValidationFailed {
		t.Fatalf("unexpected error code: %s err=%v", internalerrors.GetCode(err), err)
	}
}

func TestClient_Submit_RetriesOn429_RespectsRetryAfterSeconds(t *testing.T) {
	now := time.Date(2026, 2, 24, 10, 0, 0, 0, time.UTC)

	attempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts == 1 {
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusTooManyRequests)
			if _, err := w.Write([]byte(`{"error":"rate limited"}`)); err != nil {
				return
			}
			return
		}
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write([]byte(`{"ok":true}`)); err != nil {
			return
		}
	}))
	t.Cleanup(srv.Close)

	var sleeps []time.Duration
	c, err := upstream.NewClient(config.Config{
		Credentials: config.Credentials{AccessKey: "ak", SecretKey: "sk"},
		Region:      "cn-north-1",
		Host:        srv.URL,
		Timeout:     2 * time.Second,
	}, upstream.Options{
		Now: func() time.Time { return now },
		Sleep: func(_ context.Context, d time.Duration) error {
			sleeps = append(sleeps, d)
			return nil
		},
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	resp, callErr := c.Submit(context.Background(), []byte(`{"prompt":"cat"}`), nil)
	if callErr != nil {
		t.Fatalf("Submit unexpected error: %v", callErr)
	}
	if resp == nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected resp: %+v err=%v", resp, callErr)
	}
	if attempts != 2 {
		t.Fatalf("expected 2 attempts, got %d", attempts)
	}
	if len(sleeps) != 1 || sleeps[0] != time.Second {
		t.Fatalf("expected one sleep of 1s, got %v", sleeps)
	}
}

func TestClient_GetResult_RetriesOn5xx_RespectsRetryAfterDate(t *testing.T) {
	now := time.Date(2026, 2, 24, 10, 0, 0, 0, time.UTC)
	retryAt := now.Add(3 * time.Second)

	attempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts == 1 {
			w.Header().Set("Retry-After", retryAt.Format(http.TimeFormat))
			w.WriteHeader(http.StatusBadGateway)
			if _, err := w.Write([]byte(`{"error":"bad gateway"}`)); err != nil {
				return
			}
			return
		}
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write([]byte(`{"status":"done"}`)); err != nil {
			return
		}
	}))
	t.Cleanup(srv.Close)

	var sleeps []time.Duration
	c, err := upstream.NewClient(config.Config{
		Credentials: config.Credentials{AccessKey: "ak", SecretKey: "sk"},
		Region:      "cn-north-1",
		Host:        srv.URL,
		Timeout:     2 * time.Second,
	}, upstream.Options{
		Now: func() time.Time { return now },
		Sleep: func(_ context.Context, d time.Duration) error {
			sleeps = append(sleeps, d)
			return nil
		},
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	resp, callErr := c.GetResult(context.Background(), []byte(`{"task_id":"t1"}`), nil)
	if callErr != nil {
		t.Fatalf("GetResult unexpected error: %v", callErr)
	}
	if resp == nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected resp: %+v err=%v", resp, callErr)
	}
	if attempts != 2 {
		t.Fatalf("expected 2 attempts, got %d", attempts)
	}
	if len(sleeps) != 1 || sleeps[0] != 3*time.Second {
		t.Fatalf("expected one sleep of 3s, got %v", sleeps)
	}
}

func TestClient_Submit_NonRetriable4xx_DoesNotRetry(t *testing.T) {
	now := time.Date(2026, 2, 24, 10, 0, 0, 0, time.UTC)

	attempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.Header().Set("Retry-After", "10")
		w.WriteHeader(http.StatusBadRequest)
		if _, err := w.Write([]byte(`{"error":"bad request"}`)); err != nil {
			return
		}
	}))
	t.Cleanup(srv.Close)

	var sleeps []time.Duration
	c, err := upstream.NewClient(config.Config{
		Credentials: config.Credentials{AccessKey: "ak", SecretKey: "sk"},
		Region:      "cn-north-1",
		Host:        srv.URL,
		Timeout:     2 * time.Second,
	}, upstream.Options{
		Now: func() time.Time { return now },
		Sleep: func(_ context.Context, d time.Duration) error {
			sleeps = append(sleeps, d)
			return nil
		},
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	resp, callErr := c.Submit(context.Background(), []byte(`{"prompt":"cat"}`), nil)
	if callErr == nil {
		t.Fatalf("expected error")
	}
	if resp == nil || resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("unexpected resp: %+v", resp)
	}
	if attempts != 1 {
		t.Fatalf("expected 1 attempt, got %d", attempts)
	}
	if len(sleeps) != 0 {
		t.Fatalf("expected no sleep, got %v", sleeps)
	}
}

func TestClient_Submit_BoundedBackoffAndMaxRetries(t *testing.T) {
	now := time.Date(2026, 2, 24, 10, 0, 0, 0, time.UTC)

	attempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.WriteHeader(http.StatusServiceUnavailable)
		if _, err := w.Write([]byte(strconv.Itoa(attempts))); err != nil {
			return
		}
	}))
	t.Cleanup(srv.Close)

	var sleeps []time.Duration
	c, err := upstream.NewClient(config.Config{
		Credentials: config.Credentials{AccessKey: "ak", SecretKey: "sk"},
		Region:      "cn-north-1",
		Host:        srv.URL,
		Timeout:     2 * time.Second,
	}, upstream.Options{
		Now:        func() time.Time { return now },
		MaxRetries: 2,
		Sleep: func(_ context.Context, d time.Duration) error {
			sleeps = append(sleeps, d)
			return nil
		},
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	resp, callErr := c.Submit(context.Background(), []byte(`{"prompt":"cat"}`), nil)
	if callErr == nil {
		t.Fatalf("expected error")
	}
	if resp == nil || resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("unexpected resp: %+v", resp)
	}
	if attempts != 3 {
		t.Fatalf("expected 3 attempts, got %d", attempts)
	}
	if len(sleeps) != 2 {
		t.Fatalf("expected two sleeps, got %v", sleeps)
	}
	if sleeps[0] != 200*time.Millisecond || sleeps[1] != 400*time.Millisecond {
		t.Fatalf("unexpected backoff sequence: %v", sleeps)
	}
}

func TestClient_Submit_RespectsSubmitMinInterval(t *testing.T) {
	now := time.Date(2026, 2, 24, 10, 0, 0, 0, time.UTC)

	attempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write([]byte(`{"ok":true}`)); err != nil {
			return
		}
	}))
	t.Cleanup(srv.Close)

	var sleeps []time.Duration
	c, err := upstream.NewClient(config.Config{
		Credentials: config.Credentials{AccessKey: "ak", SecretKey: "sk"},
		Region:      "cn-north-1",
		Host:        srv.URL,
		Timeout:     2 * time.Second,
	}, upstream.Options{
		Now: func() time.Time { return now },
		Sleep: func(_ context.Context, d time.Duration) error {
			sleeps = append(sleeps, d)
			now = now.Add(d)
			return nil
		},
		SubmitMinInterval: 1500 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	if _, err := c.Submit(context.Background(), []byte(`{"prompt":"cat"}`), nil); err != nil {
		t.Fatalf("first Submit unexpected error: %v", err)
	}
	if _, err := c.Submit(context.Background(), []byte(`{"prompt":"cat"}`), nil); err != nil {
		t.Fatalf("second Submit unexpected error: %v", err)
	}

	if attempts != 2 {
		t.Fatalf("expected 2 attempts, got %d", attempts)
	}
	if len(sleeps) != 1 || sleeps[0] != 1500*time.Millisecond {
		t.Fatalf("expected one submit interval sleep of 1.5s, got %v", sleeps)
	}
}

func TestClient_GetResult_WaitsForGlobalQueue(t *testing.T) {
	submitStarted := make(chan struct{})
	releaseSubmit := make(chan struct{})
	releasedSubmit := false
	releaseSubmitOnce := func() {
		if releasedSubmit {
			return
		}
		close(releaseSubmit)
		releasedSubmit = true
	}
	defer releaseSubmitOnce()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		action := r.URL.Query().Get("Action")
		switch action {
		case "CVSync2AsyncSubmitTask":
			close(submitStarted)
			<-releaseSubmit
			w.WriteHeader(http.StatusOK)
			if _, err := w.Write([]byte(`{"ok":true}`)); err != nil {
				return
			}
		case "CVSync2AsyncGetResult":
			w.WriteHeader(http.StatusOK)
			if _, err := w.Write([]byte(`{"status":"running"}`)); err != nil {
				return
			}
		default:
			w.WriteHeader(http.StatusBadRequest)
		}
	}))
	t.Cleanup(srv.Close)

	c, err := upstream.NewClient(config.Config{
		Credentials: config.Credentials{AccessKey: "ak", SecretKey: "sk"},
		Region:      "cn-north-1",
		Host:        srv.URL,
		Timeout:     2 * time.Second,
	}, upstream.Options{
		SubmitMinInterval: 2 * time.Second,
		MaxConcurrent:     1,
		MaxQueue:          10,
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	submitErrCh := make(chan error, 1)
	go func() {
		_, submitErr := c.Submit(context.Background(), []byte(`{"prompt":"cat"}`), nil)
		submitErrCh <- submitErr
	}()

	select {
	case <-submitStarted:
	case <-time.After(2 * time.Second):
		t.Fatalf("submit did not reach upstream in time")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	resp, getErr := c.GetResult(ctx, []byte(`{"task_id":"t1"}`), nil)
	if getErr == nil {
		t.Fatalf("expected get-result to wait for global queue and timeout")
	}
	if internalerrors.GetCode(getErr) != internalerrors.ErrUpstreamFailed {
		t.Fatalf("expected UPSTREAM_FAILED, got code=%s err=%v", internalerrors.GetCode(getErr), getErr)
	}
	if resp != nil {
		t.Fatalf("expected nil response when waiting in queue times out, got %+v", resp)
	}

	releaseSubmitOnce()
	if submitErr := <-submitErrCh; submitErr != nil {
		t.Fatalf("Submit unexpected error: %v", submitErr)
	}
}

func TestClient_Submit_GlobalQueueFull_ReturnsErrRateLimitedImmediately(t *testing.T) {
	started := make(chan struct{})
	releaseFirst := make(chan struct{})
	var releaseFirstOnce sync.Once
	releaseFirstGate := func() {
		releaseFirstOnce.Do(func() {
			close(releaseFirst)
		})
	}
	defer releaseFirstGate()

	var requestCount int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("Action") != "CVSync2AsyncSubmitTask" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		atomic.AddInt32(&requestCount, 1)
		select {
		case <-started:
		default:
			close(started)
		}
		<-releaseFirst
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write([]byte(`{"ok":true}`)); err != nil {
			return
		}
	}))
	t.Cleanup(srv.Close)

	c, err := upstream.NewClient(config.Config{
		Credentials: config.Credentials{AccessKey: "ak", SecretKey: "sk"},
		Region:      "cn-north-1",
		Host:        srv.URL,
		Timeout:     2 * time.Second,
	}, upstream.Options{
		MaxConcurrent: 1,
		MaxQueue:      1,
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	firstDone := make(chan error, 1)
	go func() {
		_, submitErr := c.Submit(context.Background(), []byte(`{"req_key":"jimeng_video_v30"}`), nil)
		firstDone <- submitErr
	}()

	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatalf("first submit did not reach upstream in time")
	}

	type callResult struct {
		resp *upstream.Response
		err  error
	}
	secondDone := make(chan callResult, 1)
	go func() {
		resp, submitErr := c.Submit(context.Background(), []byte(`{"req_key":"jimeng_video_v30"}`), nil)
		secondDone <- callResult{resp: resp, err: submitErr}
	}()

	waitForUpstreamWaitersLen(t, c, 1)

	thirdDone := make(chan callResult, 1)
	go func() {
		resp, submitErr := c.Submit(context.Background(), []byte(`{"req_key":"jimeng_video_v30"}`), nil)
		thirdDone <- callResult{resp: resp, err: submitErr}
	}()

	select {
	case out := <-thirdDone:
		if out.err == nil {
			t.Fatalf("expected third submit to be rate limited by full queue")
		}
		if out.resp != nil {
			t.Fatalf("expected nil response on queue full rate limit, got %+v", out.resp)
		}
		if internalerrors.GetCode(out.err) != internalerrors.ErrRateLimited {
			t.Fatalf("expected RATE_LIMITED, got code=%s err=%v", internalerrors.GetCode(out.err), out.err)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatalf("queue-full rejection should be immediate, but third request did not finish in time")
	}

	if got := atomic.LoadInt32(&requestCount); got != 1 {
		t.Fatalf("expected only one upstream request, got %d", got)
	}

	releaseFirstGate()
	if err := <-firstDone; err != nil {
		t.Fatalf("first submit unexpected error: %v", err)
	}

	out2 := <-secondDone
	if out2.err != nil {
		t.Fatalf("second (queued) submit unexpected error: %v", out2.err)
	}
	if out2.resp == nil || out2.resp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected second response: %+v", out2.resp)
	}
	if got := atomic.LoadInt32(&requestCount); got != 2 {
		t.Fatalf("expected two upstream requests (first + dequeued second), got %d", got)
	}
}

func TestClient_Submit_SameAPIKeySecondConcurrentRequestRateLimitedImmediately(t *testing.T) {
	started := make(chan struct{})
	releaseFirst := make(chan struct{})
	var releaseFirstOnce sync.Once
	releaseFirstGate := func() {
		releaseFirstOnce.Do(func() {
			close(releaseFirst)
		})
	}
	defer releaseFirstGate()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("Action") != "CVSync2AsyncSubmitTask" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		select {
		case <-started:
		default:
			close(started)
		}
		<-releaseFirst
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write([]byte(`{"ok":true}`)); err != nil {
			return
		}
	}))
	t.Cleanup(srv.Close)

	c, err := upstream.NewClient(config.Config{
		Credentials: config.Credentials{AccessKey: "ak", SecretKey: "sk"},
		Region:      "cn-north-1",
		Host:        srv.URL,
		Timeout:     2 * time.Second,
	}, upstream.Options{
		MaxConcurrent: 2,
		MaxQueue:      10,
		KeyManager:    keymanager.NewService(nil),
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	ctxSameKey := upstream.WithAPIKeyID(context.Background(), "key_same")
	firstErrCh := make(chan error, 1)
	go func() {
		_, submitErr := c.Submit(ctxSameKey, []byte(`{"prompt":"cat"}`), nil)
		firstErrCh <- submitErr
	}()

	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatalf("first submit did not reach upstream in time")
	}

	secondDone := make(chan struct {
		resp *upstream.Response
		err  error
	}, 1)
	go func() {
		resp, err := c.Submit(ctxSameKey, []byte(`{"prompt":"dog"}`), nil)
		secondDone <- struct {
			resp *upstream.Response
			err  error
		}{resp: resp, err: err}
	}()

	select {
	case out := <-secondDone:
		if out.err == nil {
			t.Fatalf("expected second submit to be rate limited")
		}
		if out.resp != nil {
			t.Fatalf("expected nil response on rate limit, got %+v", out.resp)
		}
		if internalerrors.GetCode(out.err) != internalerrors.ErrRateLimited {
			t.Fatalf("expected RATE_LIMITED, got code=%s err=%v", internalerrors.GetCode(out.err), out.err)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatalf("same-key rejection should be immediate, but second request did not finish in time")
	}

	releaseFirstGate()
	if firstErr := <-firstErrCh; firstErr != nil {
		t.Fatalf("first submit unexpected error: %v", firstErr)
	}
}

func TestClient_Submit_KeyRevokedRejectsNewRequestsButDoesNotCancelInFlight(t *testing.T) {
	started := make(chan struct{})
	releaseFirst := make(chan struct{})
	var releaseFirstOnce sync.Once
	releaseFirstGate := func() {
		releaseFirstOnce.Do(func() {
			close(releaseFirst)
		})
	}
	defer releaseFirstGate()

	var requestCount int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("Action") != "CVSync2AsyncSubmitTask" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		atomic.AddInt32(&requestCount, 1)
		select {
		case <-started:
		default:
			close(started)
		}
		<-releaseFirst
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write([]byte(`{"ok":true}`)); err != nil {
			return
		}
	}))
	t.Cleanup(srv.Close)

	km := keymanager.NewService(nil)
	c, err := upstream.NewClient(config.Config{
		Credentials: config.Credentials{AccessKey: "ak", SecretKey: "sk"},
		Region:      "cn-north-1",
		Host:        srv.URL,
		Timeout:     2 * time.Second,
	}, upstream.Options{
		MaxConcurrent: 2,
		MaxQueue:      10,
		KeyManager:    km,
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	type callResult struct {
		resp *upstream.Response
		err  error
	}
	ctx := upstream.WithAPIKeyID(context.Background(), "key_revoked")
	firstCh := make(chan callResult, 1)
	go func() {
		resp, callErr := c.Submit(ctx, []byte(`{"prompt":"cat"}`), nil)
		firstCh <- callResult{resp: resp, err: callErr}
	}()

	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatalf("first submit did not reach upstream in time")
	}

	km.RevokeKey("key_revoked")

	secondDone := make(chan struct {
		resp *upstream.Response
		err  error
	}, 1)
	go func() {
		resp, err := c.Submit(ctx, []byte(`{"prompt":"dog"}`), nil)
		secondDone <- struct {
			resp *upstream.Response
			err  error
		}{resp: resp, err: err}
	}()

	select {
	case out := <-secondDone:
		if out.err == nil {
			t.Fatalf("expected second submit to be rejected after revoke")
		}
		if out.resp != nil {
			t.Fatalf("expected nil response on key revoked, got %+v", out.resp)
		}
		if internalerrors.GetCode(out.err) != internalerrors.ErrKeyRevoked {
			t.Fatalf("expected KEY_REVOKED, got code=%s err=%v", internalerrors.GetCode(out.err), out.err)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatalf("revoked-key rejection should be immediate, but second request did not finish in time")
	}

	if got := atomic.LoadInt32(&requestCount); got != 1 {
		t.Fatalf("expected only one upstream request, got %d", got)
	}

	releaseFirstGate()
	first := <-firstCh
	if first.err != nil {
		t.Fatalf("first submit unexpected error: %v", first.err)
	}
	if first.resp == nil || first.resp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected first resp: %+v", first.resp)
	}
}

func TestClient_GetResult_DifferentAPIKeysProgressUnderGlobalFIFO(t *testing.T) {
	started := make(chan struct{})
	releaseFirst := make(chan struct{})
	secondReachedUpstream := make(chan struct{})
	var releaseFirstOnce sync.Once
	releaseFirstGate := func() {
		releaseFirstOnce.Do(func() {
			close(releaseFirst)
		})
	}
	defer releaseFirstGate()

	var orderMu sync.Mutex
	order := make([]string, 0, 2)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("Action") != "CVSync2AsyncGetResult" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		payload, err := io.ReadAll(r.Body)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		taskID := ""
		var reqBody map[string]any
		if err := json.Unmarshal(payload, &reqBody); err == nil {
			if v, ok := reqBody["task_id"].(string); ok {
				taskID = v
			}
		}

		orderMu.Lock()
		order = append(order, taskID)
		orderMu.Unlock()

		select {
		case <-started:
		default:
			close(started)
			<-releaseFirst
			break
		}

		if taskID == "t2" {
			close(secondReachedUpstream)
		}
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write([]byte(`{"status":"done"}`)); err != nil {
			return
		}
	}))
	t.Cleanup(srv.Close)

	c, err := upstream.NewClient(config.Config{
		Credentials: config.Credentials{AccessKey: "ak", SecretKey: "sk"},
		Region:      "cn-north-1",
		Host:        srv.URL,
		Timeout:     2 * time.Second,
	}, upstream.Options{
		MaxConcurrent: 1,
		MaxQueue:      10,
		KeyManager:    keymanager.NewService(nil),
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	firstCtx := upstream.WithAPIKeyID(context.Background(), "key_a")
	secondCtx := upstream.WithAPIKeyID(context.Background(), "key_b")

	firstErrCh := make(chan error, 1)
	go func() {
		_, getErr := c.GetResult(firstCtx, []byte(`{"task_id":"t1"}`), nil)
		firstErrCh <- getErr
	}()

	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatalf("first get-result did not reach upstream in time")
	}

	secondDone := make(chan error, 1)
	go func() {
		_, getErr := c.GetResult(secondCtx, []byte(`{"task_id":"t2"}`), nil)
		secondDone <- getErr
	}()

	waitForUpstreamWaitersLen(t, c, 1)
	select {
	case <-secondReachedUpstream:
		t.Fatalf("second get-result should not reach upstream before first finishes")
	default:
	}

	releaseFirstGate()
	if firstErr := <-firstErrCh; firstErr != nil {
		t.Fatalf("first get-result unexpected error: %v", firstErr)
	}

	select {
	case <-secondReachedUpstream:
	case <-time.After(2 * time.Second):
		t.Fatalf("second get-result did not reach upstream after first completed")
	}

	if secondErr := <-secondDone; secondErr != nil {
		t.Fatalf("second get-result unexpected error: %v", secondErr)
	}

	orderMu.Lock()
	gotOrder := append([]string(nil), order...)
	orderMu.Unlock()
	if !reflect.DeepEqual(gotOrder, []string{"t1", "t2"}) {
		t.Fatalf("expected global FIFO order [t1 t2], got %v", gotOrder)
	}
}

func TestClient_GetResult_HandoffReservationCannotBeStolenByThirdAcquire(t *testing.T) {
	prevProcs := runtime.GOMAXPROCS(1)
	defer runtime.GOMAXPROCS(prevProcs)

	const attempts = 30
	for i := 0; i < attempts; i++ {
		t.Run("attempt_"+strconv.Itoa(i), func(t *testing.T) {
			firstStarted := make(chan struct{})
			releaseFirst := make(chan struct{})
			secondReachedUpstream := make(chan struct{})
			thirdReachedUpstream := make(chan struct{})
			var releaseFirstOnce sync.Once
			releaseFirstGate := func() {
				releaseFirstOnce.Do(func() {
					close(releaseFirst)
				})
			}
			defer releaseFirstGate()

			var orderMu sync.Mutex
			order := make([]string, 0, 3)

			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Query().Get("Action") != "CVSync2AsyncGetResult" {
					w.WriteHeader(http.StatusBadRequest)
					return
				}

				payload, err := io.ReadAll(r.Body)
				if err != nil {
					w.WriteHeader(http.StatusInternalServerError)
					return
				}

				taskID := ""
				var reqBody map[string]any
				if err := json.Unmarshal(payload, &reqBody); err == nil {
					if v, ok := reqBody["task_id"].(string); ok {
						taskID = v
					}
				}

				orderMu.Lock()
				order = append(order, taskID)
				orderMu.Unlock()

				switch taskID {
				case "t1":
					select {
					case <-firstStarted:
					default:
						close(firstStarted)
					}
					<-releaseFirst
				case "t2":
					close(secondReachedUpstream)
				case "t3":
					close(thirdReachedUpstream)
				}

				w.WriteHeader(http.StatusOK)
				if _, err := w.Write([]byte(`{"status":"done"}`)); err != nil {
					return
				}
			}))
			t.Cleanup(srv.Close)

			c, err := upstream.NewClient(config.Config{
				Credentials: config.Credentials{AccessKey: "ak", SecretKey: "sk"},
				Region:      "cn-north-1",
				Host:        srv.URL,
				Timeout:     2 * time.Second,
			}, upstream.Options{
				MaxConcurrent: 1,
				MaxQueue:      10,
				KeyManager:    keymanager.NewService(nil),
			})
			if err != nil {
				t.Fatalf("NewClient: %v", err)
			}

			firstCtx := upstream.WithAPIKeyID(context.Background(), "key_a")
			secondCtx := upstream.WithAPIKeyID(context.Background(), "key_b")
			thirdCtx := upstream.WithAPIKeyID(context.Background(), "key_c")

			firstDone := make(chan error, 1)
			go func() {
				ctx, cancel := context.WithTimeout(firstCtx, 2*time.Second)
				defer cancel()
				_, getErr := c.GetResult(ctx, []byte(`{"task_id":"t1"}`), nil)
				firstDone <- getErr
			}()

			select {
			case <-firstStarted:
			case <-time.After(2 * time.Second):
				t.Fatalf("first get-result did not reach upstream in time")
			}

			secondDone := make(chan error, 1)
			go func() {
				ctx, cancel := context.WithTimeout(secondCtx, 2*time.Second)
				defer cancel()
				_, getErr := c.GetResult(ctx, []byte(`{"task_id":"t2"}`), nil)
				secondDone <- getErr
			}()

			waitForUpstreamWaitersLen(t, c, 1)

			raceStart := make(chan struct{})
			thirdDone := make(chan error, 1)
			go func() {
				<-raceStart
				ctx, cancel := context.WithTimeout(thirdCtx, 2*time.Second)
				defer cancel()
				_, getErr := c.GetResult(ctx, []byte(`{"task_id":"t3"}`), nil)
				thirdDone <- getErr
			}()
			go func() {
				<-raceStart
				releaseFirstGate()
			}()

			close(raceStart)

			select {
			case <-thirdReachedUpstream:
				t.Fatalf("third request reached upstream before handed-off waiter")
			case <-secondReachedUpstream:
			}

			if firstErr := <-firstDone; firstErr != nil {
				t.Fatalf("first get-result unexpected error: %v", firstErr)
			}
			if secondErr := <-secondDone; secondErr != nil {
				t.Fatalf("second get-result unexpected error: %v", secondErr)
			}
			if thirdErr := <-thirdDone; thirdErr != nil {
				t.Fatalf("third get-result unexpected error: %v", thirdErr)
			}

			orderMu.Lock()
			gotOrder := append([]string(nil), order...)
			orderMu.Unlock()
			if !reflect.DeepEqual(gotOrder, []string{"t1", "t2", "t3"}) {
				t.Fatalf("expected FIFO order [t1 t2 t3], got %v", gotOrder)
			}
		})
	}
}

func TestClient_CancelledQueuedRequest_ReleasesPerKeyGate(t *testing.T) {
	started := make(chan struct{})
	releaseFirst := make(chan struct{})
	var requestCount int32
	var releaseFirstOnce sync.Once
	releaseFirstGate := func() {
		releaseFirstOnce.Do(func() {
			close(releaseFirst)
		})
	}
	defer releaseFirstGate()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("Action") != "CVSync2AsyncGetResult" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		atomic.AddInt32(&requestCount, 1)
		select {
		case <-started:
		default:
			close(started)
			<-releaseFirst
		}
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write([]byte(`{"status":"done"}`)); err != nil {
			return
		}
	}))
	t.Cleanup(srv.Close)

	c, err := upstream.NewClient(config.Config{
		Credentials: config.Credentials{AccessKey: "ak", SecretKey: "sk"},
		Region:      "cn-north-1",
		Host:        srv.URL,
		Timeout:     2 * time.Second,
	}, upstream.Options{
		MaxConcurrent: 1,
		MaxQueue:      10,
		KeyManager:    keymanager.NewService(nil),
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	firstCtx := upstream.WithAPIKeyID(context.Background(), "key_a")
	firstDone := make(chan error, 1)
	go func() {
		_, getErr := c.GetResult(firstCtx, []byte(`{"task_id":"t1"}`), nil)
		firstDone <- getErr
	}()

	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatalf("first get-result did not reach upstream in time")
	}

	queuedCtx, cancelQueued := context.WithCancel(upstream.WithAPIKeyID(context.Background(), "key_b"))
	queuedDone := make(chan error, 1)
	go func() {
		_, getErr := c.GetResult(queuedCtx, []byte(`{"task_id":"t2"}`), nil)
		queuedDone <- getErr
	}()

	waitForUpstreamWaitersLen(t, c, 1)
	cancelQueued()

	queuedErr := <-queuedDone
	if queuedErr == nil {
		t.Fatalf("expected queued request cancellation error")
	}
	if internalerrors.GetCode(queuedErr) != internalerrors.ErrUpstreamFailed {
		t.Fatalf("expected UPSTREAM_FAILED, got code=%s err=%v", internalerrors.GetCode(queuedErr), queuedErr)
	}
	waitForUpstreamWaitersLen(t, c, 0)

	followupCtx := upstream.WithAPIKeyID(context.Background(), "key_b")
	followupDone := make(chan error, 1)
	go func() {
		_, getErr := c.GetResult(followupCtx, []byte(`{"task_id":"t3"}`), nil)
		followupDone <- getErr
	}()

	waitForUpstreamWaitersLen(t, c, 1)

	releaseFirstGate()
	if firstErr := <-firstDone; firstErr != nil {
		t.Fatalf("first get-result unexpected error: %v", firstErr)
	}
	if followupErr := <-followupDone; followupErr != nil {
		t.Fatalf("follow-up same-key request should succeed, got err=%v", followupErr)
	}
	if got := atomic.LoadInt32(&requestCount); got != 2 {
		t.Fatalf("expected two upstream calls (first + follow-up), got %d", got)
	}
	waitForUpstreamWaitersLen(t, c, 0)
}

func TestClient_GetResult_TimeoutWhileQueued_DoesNotLeakWaiters(t *testing.T) {
	started := make(chan struct{})
	releaseFirst := make(chan struct{})
	releasedFirst := false
	releaseFirstOnce := func() {
		if releasedFirst {
			return
		}
		close(releaseFirst)
		releasedFirst = true
	}
	defer releaseFirstOnce()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("Action") != "CVSync2AsyncGetResult" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		select {
		case <-started:
		default:
			close(started)
			<-releaseFirst
		}
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write([]byte(`{"status":"done"}`)); err != nil {
			return
		}
	}))
	t.Cleanup(srv.Close)

	c, err := upstream.NewClient(config.Config{
		Credentials: config.Credentials{AccessKey: "ak", SecretKey: "sk"},
		Region:      "cn-north-1",
		Host:        srv.URL,
		Timeout:     2 * time.Second,
	}, upstream.Options{
		MaxConcurrent: 1,
		MaxQueue:      10,
		KeyManager:    keymanager.NewService(nil),
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	firstCtx := upstream.WithAPIKeyID(context.Background(), "key_a")
	firstDone := make(chan error, 1)
	go func() {
		_, getErr := c.GetResult(firstCtx, []byte(`{"task_id":"t1"}`), nil)
		firstDone <- getErr
	}()

	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatalf("first get-result did not reach upstream in time")
	}

	baseline := upstreamWaitersLen(c)
	if baseline != 0 {
		t.Fatalf("expected baseline waiters 0, got %d", baseline)
	}

	timeoutCtx, cancelTimeout := context.WithTimeout(upstream.WithAPIKeyID(context.Background(), "key_b"), 120*time.Millisecond)
	defer cancelTimeout()

	timeoutDone := make(chan error, 1)
	go func() {
		_, getErr := c.GetResult(timeoutCtx, []byte(`{"task_id":"t2"}`), nil)
		timeoutDone <- getErr
	}()

	waitForUpstreamWaitersLen(t, c, baseline+1)

	timeoutErr := <-timeoutDone
	if timeoutErr == nil {
		t.Fatalf("expected timeout while queued")
	}
	if internalerrors.GetCode(timeoutErr) != internalerrors.ErrUpstreamFailed {
		t.Fatalf("expected UPSTREAM_FAILED, got code=%s err=%v", internalerrors.GetCode(timeoutErr), timeoutErr)
	}

	waitForUpstreamWaitersLen(t, c, baseline)

	releaseFirstOnce()
	if firstErr := <-firstDone; firstErr != nil {
		t.Fatalf("first get-result unexpected error: %v", firstErr)
	}
	waitForUpstreamWaitersLen(t, c, baseline)
}

func upstreamWaitersLen(c *upstream.Client) int {
	if c == nil {
		return 0
	}
	v := reflect.ValueOf(c).Elem().FieldByName("waiters")
	if !v.IsValid() {
		return 0
	}
	return v.Len()
}

func waitForUpstreamWaitersLen(t *testing.T, c *upstream.Client, want int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if got := upstreamWaitersLen(c); got == want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("waiters length did not reach %d, got %d", want, upstreamWaitersLen(c))
}

func verifySigV4Request(r *http.Request, expectedAK, secret, region, service string) error {
	authorization := strings.TrimSpace(r.Header.Get("Authorization"))
	if authorization == "" {
		return internalerrors.New(internalerrors.ErrInvalidSignature, "missing authorization", nil)
	}
	fields, err := parseAuthorizationHeader(authorization)
	if err != nil {
		return err
	}
	if fields.accessKey != expectedAK {
		return internalerrors.New(internalerrors.ErrInvalidSignature, "unexpected access key", nil)
	}
	if fields.region != region || fields.service != service {
		return internalerrors.New(internalerrors.ErrInvalidSignature, "unexpected credential scope", nil)
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		return internalerrors.New(internalerrors.ErrInvalidSignature, "read body", err)
	}
	r.Body = io.NopCloser(bytes.NewReader(body))
	payloadHash := sha256Hex(body)
	if got := strings.ToLower(strings.TrimSpace(r.Header.Get("X-Content-Sha256"))); got != payloadHash {
		return internalerrors.New(internalerrors.ErrInvalidSignature, "payload hash mismatch", nil)
	}
	xDate := strings.TrimSpace(r.Header.Get("X-Date"))
	if xDate == "" {
		return internalerrors.New(internalerrors.ErrInvalidSignature, "missing x-date", nil)
	}

	canonicalRequest, err := buildCanonicalRequest(r, fields.signedHeaders, payloadHash)
	if err != nil {
		return err
	}
	scope := strings.Join([]string{fields.dateScope, fields.region, fields.service, fields.scopeSuffix}, "/")
	algorithm := fields.algorithm
	if algorithm == "" {
		algorithm = "AWS4-HMAC-SHA256"
	}
	stringToSign := strings.Join([]string{
		algorithm,
		xDate,
		scope,
		sha256Hex([]byte(canonicalRequest)),
	}, "\n")
	signingKey := deriveSigningKey(secret, fields.dateScope, fields.region, fields.service, fields.scopeSuffix)
	expectedSignature := hex.EncodeToString(hmacSHA256(signingKey, stringToSign))
	if !strings.EqualFold(fields.signature, expectedSignature) {
		return internalerrors.New(internalerrors.ErrInvalidSignature, "signature mismatch", nil)
	}
	return nil
}

type authFields struct {
	accessKey     string
	dateScope     string
	region        string
	service       string
	scopeSuffix   string
	algorithm     string
	signedHeaders []string
	signature     string
}

func parseAuthorizationHeader(v string) (authFields, error) {
	algorithm := ""
	body := ""
	if strings.HasPrefix(v, "AWS4-HMAC-SHA256 ") {
		algorithm = "AWS4-HMAC-SHA256"
		body = strings.TrimSpace(strings.TrimPrefix(v, "AWS4-HMAC-SHA256 "))
	} else if strings.HasPrefix(v, "HMAC-SHA256 ") {
		algorithm = "HMAC-SHA256"
		body = strings.TrimSpace(strings.TrimPrefix(v, "HMAC-SHA256 "))
	} else {
		return authFields{}, internalerrors.New(internalerrors.ErrInvalidSignature, "unsupported authorization algorithm", nil)
	}
	parts := strings.Split(body, ",")
	values := map[string]string{}
	for _, part := range parts {
		kv := strings.SplitN(strings.TrimSpace(part), "=", 2)
		if len(kv) != 2 {
			return authFields{}, internalerrors.New(internalerrors.ErrInvalidSignature, "invalid authorization segment", nil)
		}
		values[kv[0]] = kv[1]
	}
	credential := strings.TrimSpace(values["Credential"])
	signedHeaders := strings.TrimSpace(values["SignedHeaders"])
	signature := strings.TrimSpace(values["Signature"])
	if credential == "" || signedHeaders == "" || signature == "" {
		return authFields{}, internalerrors.New(internalerrors.ErrInvalidSignature, "authorization fields are incomplete", nil)
	}
	scope := strings.Split(credential, "/")
	if len(scope) != 5 || (scope[4] != "aws4_request" && scope[4] != "request") {
		return authFields{}, internalerrors.New(internalerrors.ErrInvalidSignature, "invalid credential scope", nil)
	}
	parsedHeaders := strings.Split(strings.ToLower(signedHeaders), ";")
	for i := range parsedHeaders {
		parsedHeaders[i] = strings.TrimSpace(parsedHeaders[i])
	}
	if scope[0] == "" || scope[1] == "" || scope[2] == "" || scope[3] == "" {
		return authFields{}, internalerrors.New(internalerrors.ErrInvalidSignature, "credential scope is incomplete", nil)
	}
	return authFields{
		accessKey:     scope[0],
		dateScope:     scope[1],
		region:        scope[2],
		service:       scope[3],
		scopeSuffix:   scope[4],
		algorithm:     algorithm,
		signedHeaders: parsedHeaders,
		signature:     strings.ToLower(signature),
	}, nil
}

func buildCanonicalRequest(r *http.Request, signedHeaders []string, payloadHash string) (string, error) {
	headers := append([]string(nil), signedHeaders...)
	sort.Strings(headers)
	canonHeaders := strings.Builder{}
	for _, h := range headers {
		if strings.TrimSpace(h) == "" {
			return "", internalerrors.New(internalerrors.ErrInvalidSignature, "empty signed header", nil)
		}
		v := canonicalHeaderValue(r, h)
		if v == "" {
			return "", internalerrors.New(internalerrors.ErrInvalidSignature, "missing signed header: "+h, nil)
		}
		canonHeaders.WriteString(h)
		canonHeaders.WriteByte(':')
		canonHeaders.WriteString(v)
		canonHeaders.WriteByte('\n')
	}

	canon := strings.Join([]string{
		r.Method,
		canonicalURI(r.URL.Path),
		canonicalQueryString(r.URL.Query()),
		canonHeaders.String(),
		strings.Join(headers, ";"),
		payloadHash,
	}, "\n")
	return canon, nil
}

func canonicalHeaderValue(r *http.Request, name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "host" {
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
	var kDate []byte
	if suffix == "request" {
		kDate = hmacSHA256([]byte(secret), date)
	} else {
		kDate = hmacSHA256([]byte("AWS4"+secret), date)
	}
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

func TestClient_UpstreamParity(t *testing.T) {
	// This test asserts parity between the direct Volcengine SDK call and our relay upstream client.
	// It is currently expected to FAIL (RED phase) to document the parity gaps.

	now := time.Date(2026, 2, 24, 10, 0, 0, 0, time.UTC)
	ak := "ak_test"
	sk := "sk_test"
	region := "cn-north-1"

	tests := []struct {
		name    string
		reqBody map[string]any
	}{
		{
			name: "t2v-720",
			reqBody: map[string]any{
				"req_key":    "jimeng_t2v_v30_720p",
				"prompt":     "a beautiful cat",
				"resolution": "1280x720",
			},
		},
		{
			name: "i2v-first",
			reqBody: map[string]any{
				"req_key":   "jimeng_i2v_first_v30_1080",
				"prompt":    "make the cat smile",
				"image_url": "https://example.com/cat.png",
			},
		},
		{
			name: "i2v-first-tail",
			reqBody: map[string]any{
				"req_key":        "jimeng_i2v_first_tail_v30_1080",
				"prompt":         "cat running",
				"image_url":      "https://example.com/start.png",
				"tail_image_url": "https://example.com/end.png",
			},
		},
		{
			name: "missing-req-key",
			reqBody: map[string]any{
				"prompt": "a cat",
			},
		},

	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			type capturedRequest struct {
				Method string
				URL    *url.URL
				Header http.Header
				Body   []byte
			}

			var captured []capturedRequest
			var mu sync.Mutex

			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, _ := io.ReadAll(r.Body)
				mu.Lock()
				captured = append(captured, capturedRequest{
					Method: r.Method,
					URL:    r.URL,
					Header: r.Header.Clone(),
					Body:   body,
				})
				mu.Unlock()
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{"ResponseMetadata":{"RequestId":"test-req-id"},"Result":{"task_id":"test-task-id"}}`))
			}))
			defer srv.Close()

			u, _ := url.Parse(srv.URL)

			// 1. Direct SDK Call
			v := visual.NewInstance()
			v.Client.SetAccessKey(ak)
			v.Client.SetSecretKey(sk)
			v.SetRegion(region)
			v.SetHost(u.Host)
			v.SetSchema("http")

			_, _, _ = v.CVSync2AsyncSubmitTask(tt.reqBody)

			// 2. Relay Upstream Call
			c, _ := upstream.NewClient(config.Config{
				Credentials: config.Credentials{AccessKey: ak, SecretKey: sk},
				Region:      region,
				Host:        srv.URL,
			}, upstream.Options{Now: func() time.Time { return now }})

			relayBody, _ := json.Marshal(tt.reqBody)
			_, _ = c.Submit(context.Background(), relayBody, nil)

			if len(captured) != 2 {
				t.Fatalf("expected 2 captured requests, got %d", len(captured))
			}

			direct := captured[0]
			relay := captured[1]

			// Parity Assertions
			t.Run("ActionParity", func(t *testing.T) {
				directAction := direct.URL.Query().Get("Action")
				relayAction := relay.URL.Query().Get("Action")
				if directAction != relayAction {
					t.Errorf("Action mismatch: direct=%q relay=%q", directAction, relayAction)
				}
			})

			t.Run("VersionParity", func(t *testing.T) {
				directVersion := direct.URL.Query().Get("Version")
				relayVersion := relay.URL.Query().Get("Version")
				if directVersion != relayVersion {
					t.Errorf("Version mismatch: direct=%q relay=%q", directVersion, relayVersion)
				}
			})

			t.Run("BodyParity", func(t *testing.T) {
				var directBody, relayBody map[string]any
				_ = json.Unmarshal(direct.Body, &directBody)
				_ = json.Unmarshal(relay.Body, &relayBody)

				if !reflect.DeepEqual(directBody, relayBody) {
					t.Errorf("Body mismatch:\ndirect: %s\nrelay:  %s", string(direct.Body), string(relay.Body))
				}
			})

			t.Run("ReqKeyParity", func(t *testing.T) {
				var directBody, relayBody map[string]any
				_ = json.Unmarshal(direct.Body, &directBody)
				_ = json.Unmarshal(relay.Body, &relayBody)

				if directBody["req_key"] != relayBody["req_key"] {
					t.Errorf("req_key mismatch: direct=%v relay=%v", directBody["req_key"], relayBody["req_key"])
				}
			})

			t.Run("SigningHeadersParity", func(t *testing.T) {
				// Check if essential signing headers are present in both
				essential := []string{"Authorization", "X-Date", "X-Content-Sha256"}
				for _, h := range essential {
					if direct.Header.Get(h) == "" {
						t.Errorf("Direct request missing header %s", h)
					}
					if relay.Header.Get(h) == "" {
						t.Errorf("Relay request missing header %s", h)
					}
				}
			})
			t.Run("AuthorizationParity", func(t *testing.T) {
				directAuth := direct.Header.Get("Authorization")
				relayAuth := relay.Header.Get("Authorization")

				// Parse and compare Credential scope
				parseScope := func(auth string) string {
					parts := strings.Split(auth, " ")
					if len(parts) < 2 {
						return ""
					}
					for _, p := range strings.Split(parts[1], ",") {
						if strings.HasPrefix(strings.TrimSpace(p), "Credential=") {
							return strings.TrimPrefix(strings.TrimSpace(p), "Credential=")
						}
					}
					return ""
				}

				directScope := parseScope(directAuth)
				relayScope := parseScope(relayAuth)

				if directScope != relayScope {
					t.Errorf("Authorization Credential scope mismatch:\ndirect: %q\nrelay:  %q", directScope, relayScope)
				}
			})
			t.Run("HeaderParity", func(t *testing.T) {
				// Compare non-signing headers
				skip := map[string]bool{
					"Authorization":    true,
					"X-Date":           true,
					"X-Content-Sha256": true,
					"User-Agent":       true, // SDK might have its own UA
					"Content-Length":   true,
					"Accept-Encoding":  true,
					"Connection":       true,
				}

				for k, v := range direct.Header {
					if skip[k] {
						continue
					}
					if relay.Header.Get(k) != v[0] {
						t.Errorf("Header mismatch for %s: direct=%q relay=%q", k, v[0], relay.Header.Get(k))
					}
				}
			})


		})
	}
}
