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
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jimeng-relay/server/internal/config"
	internalerrors "github.com/jimeng-relay/server/internal/errors"
	"github.com/jimeng-relay/server/internal/relay/upstream"
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
		if !strings.HasPrefix(gotAuth, "AWS4-HMAC-SHA256 ") {
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
	scope := strings.Join([]string{fields.dateScope, fields.region, fields.service, "aws4_request"}, "/")
	stringToSign := strings.Join([]string{
		"AWS4-HMAC-SHA256",
		xDate,
		scope,
		sha256Hex([]byte(canonicalRequest)),
	}, "\n")
	signingKey := deriveSigningKey(secret, fields.dateScope, fields.region, fields.service)
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
	signedHeaders []string
	signature     string
}

func parseAuthorizationHeader(v string) (authFields, error) {
	const prefix = "AWS4-HMAC-SHA256 "
	if !strings.HasPrefix(v, prefix) {
		return authFields{}, internalerrors.New(internalerrors.ErrInvalidSignature, "unsupported authorization algorithm", nil)
	}
	body := strings.TrimSpace(strings.TrimPrefix(v, prefix))
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
	if len(scope) != 5 || scope[4] != "aws4_request" {
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

func deriveSigningKey(secret, date, region, service string) []byte {
	kDate := hmacSHA256([]byte("AWS4"+secret), date)
	kRegion := hmacSHA256(kDate, region)
	kService := hmacSHA256(kRegion, service)
	return hmacSHA256(kService, "aws4_request")
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
