package relay

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jimeng-relay/server/internal/config"
	internalerrors "github.com/jimeng-relay/server/internal/errors"
	"github.com/jimeng-relay/server/internal/middleware/sigv4"
	"github.com/jimeng-relay/server/internal/relay/upstream"
)

type fakeGetResultClient struct {
	resp *upstream.Response
	err  error

	calls      int
	reqBody    []byte
	reqHeaders http.Header
}

func (f *fakeGetResultClient) GetResult(_ context.Context, body []byte, headers http.Header) (*upstream.Response, error) {
	f.calls++
	f.reqBody = append([]byte(nil), body...)
	f.reqHeaders = headers.Clone()
	return f.resp, f.err
}

func TestGetResultHandler_PassthroughSuccess(t *testing.T) {
	upstreamBody := []byte(`{"code":10000,"message":"ok","data":{"status":"done","image_urls":["https://img.example/1.png"],"binary_data_base64":"YmFzZTY0"}}`)
	fake := &fakeGetResultClient{
		resp: &upstream.Response{
			StatusCode: http.StatusOK,
			Header: http.Header{
				"Content-Type": []string{"application/json; charset=utf-8"},
			},
			Body: upstreamBody,
		},
	}
	auditSvc, dsRepo, usRepo, aeRepo := newTestAuditService(t, nil, nil, nil)
	h := NewGetResultHandler(fake, auditSvc, slog.New(slog.NewTextHandler(io.Discard, nil))).Routes()

	requestBody := []byte(`{"task_id":"task_123"}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/get-result", bytes.NewReader(requestBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "AWS4-HMAC-SHA256 fake")
	req.Header.Set("X-Request-Id", "req-1")
	req = req.WithContext(context.WithValue(req.Context(), sigv4.ContextAPIKeyID, "k1"))
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if !bytes.Equal(rec.Body.Bytes(), upstreamBody) {
		t.Fatalf("expected body passthrough, got %q", rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); got != "application/json; charset=utf-8" {
		t.Fatalf("expected content-type passthrough, got %q", got)
	}
	if fake.calls != 1 {
		t.Fatalf("expected 1 upstream call, got %d", fake.calls)
	}
	if !bytes.Equal(fake.reqBody, requestBody) {
		t.Fatalf("expected upstream request body passthrough")
	}
	if fake.reqHeaders.Get("Authorization") != "" {
		t.Fatalf("authorization header should not be forwarded to upstream client")
	}
	if len(dsRepo.created) != 1 || len(usRepo.created) != 1 || len(aeRepo.created) != 1 {
		t.Fatalf("expected full audit chain writes, got downstream=%d upstream=%d events=%d", len(dsRepo.created), len(usRepo.created), len(aeRepo.created))
	}
	if dsRepo.created[0].RequestID != "req-1" || usRepo.created[0].RequestID != "req-1" || aeRepo.created[0].RequestID != "req-1" {
		t.Fatalf("expected request_id propagated to audit chain")
	}
	if dsRepo.created[0].Headers["Authorization"] != "***" {
		t.Fatalf("expected downstream authorization redacted")
	}
}

func TestGetResultHandler_PassthroughBusinessError(t *testing.T) {
	upstreamBody := []byte(`{"code":40011,"status":40011,"message":"invalid task_id"}`)
	fake := &fakeGetResultClient{
		resp: &upstream.Response{
			StatusCode: http.StatusBadRequest,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       upstreamBody,
		},
		err: internalerrors.New(internalerrors.ErrUpstreamFailed, "upstream get-result returned 400", nil),
	}
	auditSvc, dsRepo, usRepo, aeRepo := newTestAuditService(t, nil, nil, nil)
	h := NewGetResultHandler(fake, auditSvc, nil).Routes()

	req := httptest.NewRequest(http.MethodPost, "/v1/get-result", bytes.NewReader([]byte(`{"task_id":"invalid"}`)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Request-Id", "req-2")
	req = req.WithContext(context.WithValue(req.Context(), sigv4.ContextAPIKeyID, "k1"))
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d body=%s", rec.Code, rec.Body.String())
	}
	if !bytes.Equal(rec.Body.Bytes(), upstreamBody) {
		t.Fatalf("expected business error body passthrough, got %q", rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("expected content-type passthrough, got %q", got)
	}
	if len(dsRepo.created) != 1 || len(usRepo.created) != 1 || len(aeRepo.created) != 1 {
		t.Fatalf("expected full audit chain writes, got downstream=%d upstream=%d events=%d", len(dsRepo.created), len(usRepo.created), len(aeRepo.created))
	}
	if usRepo.created[0].ResponseStatus != http.StatusBadRequest {
		t.Fatalf("expected upstream attempt to record response status")
	}
}

func TestGetResultHandler_UpstreamNetworkError(t *testing.T) {
	fake := &fakeGetResultClient{
		err: internalerrors.New(internalerrors.ErrUpstreamFailed, "upstream request failed", errors.New("dial tcp: i/o timeout")),
	}
	auditSvc, _, _, _ := newTestAuditService(t, nil, nil, nil)
	h := NewGetResultHandler(fake, auditSvc, nil).Routes()

	req := httptest.NewRequest(http.MethodPost, "/v1/get-result", bytes.NewReader([]byte(`{"task_id":"task_123"}`)))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(context.WithValue(req.Context(), sigv4.ContextAPIKeyID, "k1"))
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("expected status 502, got %d", rec.Code)
	}
	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal response: %v", err)
	}
	errorObj, ok := payload["error"].(map[string]any)
	if !ok {
		t.Fatalf("expected error object, got %T", payload["error"])
	}
	code, ok := errorObj["code"].(string)
	if !ok {
		t.Fatalf("expected error code string, got %T", errorObj["code"])
	}
	if code != string(internalerrors.ErrUpstreamFailed) {
		t.Fatalf("expected error code %q, got %q", internalerrors.ErrUpstreamFailed, code)
	}
}

func TestGetResultHandler_CompatibleActionPath(t *testing.T) {
	upstreamBody := []byte(`{"code":10000,"message":"ok","data":{"status":"done"}}`)
	fake := &fakeGetResultClient{resp: &upstream.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: upstreamBody}}
	auditSvc, _, _, _ := newTestAuditService(t, nil, nil, nil)
	h := NewGetResultHandler(fake, auditSvc, nil).Routes()

	req := httptest.NewRequest(http.MethodPost, "/?Action=CVSync2AsyncGetResult&Version=2022-08-31", bytes.NewReader([]byte(`{"task_id":"task_123"}`)))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(context.WithValue(req.Context(), sigv4.ContextAPIKeyID, "k1"))
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
	if !bytes.Equal(rec.Body.Bytes(), upstreamBody) {
		t.Fatalf("expected body passthrough for compatible path")
	}
}

func TestGetResultHandler_AuditFailure_FailClosed(t *testing.T) {
	upstreamBody := []byte(`{"code":10000,"message":"ok","data":{"status":"done"}}`)
	fake := &fakeGetResultClient{resp: &upstream.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: upstreamBody}}
	auditSvc, _, _, _ := newTestAuditService(t, errors.New("db down"), nil, nil)
	h := NewGetResultHandler(fake, auditSvc, nil).Routes()

	req := httptest.NewRequest(http.MethodPost, "/v1/get-result", bytes.NewReader([]byte(`{"task_id":"task_123"}`)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Request-Id", "req-audit-fail")
	req = req.WithContext(context.WithValue(req.Context(), sigv4.ContextAPIKeyID, "k1"))
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d body=%s", rec.Code, rec.Body.String())
	}
	if fake.calls != 0 {
		t.Fatalf("expected audit failure to short-circuit before upstream, got calls=%d", fake.calls)
	}
	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal response: %v", err)
	}
	errorObj, ok := payload["error"].(map[string]any)
	if !ok {
		t.Fatalf("expected error object, got %T", payload["error"])
	}
	code, ok := errorObj["code"].(string)
	if !ok {
		t.Fatalf("expected error code string, got %T", errorObj["code"])
	}
	if code != string(internalerrors.ErrAuditFailed) {
		t.Fatalf("expected error code %q, got %q", internalerrors.ErrAuditFailed, code)
	}
}

func TestGetResultHandler_FakeUpstreamContract_Passthrough(t *testing.T) {
	tests := []struct {
		name                string
		upstreamStatus      int
		upstreamContentType string
		upstreamBody        []byte
	}{
		{
			name:                "success passthrough",
			upstreamStatus:      http.StatusOK,
			upstreamContentType: "application/json; charset=utf-8",
			upstreamBody:        []byte(`{"code":10000,"message":"ok","data":{"status":"done","image_urls":["https://img.example/1.png"]}}`),
		},
		{
			name:                "invalid task id business error passthrough",
			upstreamStatus:      http.StatusBadRequest,
			upstreamContentType: "application/problem+json",
			upstreamBody:        []byte(`{"code":40011,"status":40011,"message":"invalid task_id","detail":{"source":"biz-rule"}}`),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fakeUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Query().Get("Action") != "CVSync2AsyncGetResult" {
					t.Fatalf("unexpected Action query: %s", r.URL.RawQuery)
				}
				if r.Method != http.MethodPost {
					t.Fatalf("unexpected method %s", r.Method)
				}
				w.Header().Set("Content-Type", tt.upstreamContentType)
				w.WriteHeader(tt.upstreamStatus)
				if _, err := w.Write(tt.upstreamBody); err != nil {
					return
				}
			}))
			defer fakeUpstream.Close()

			c, err := upstream.NewClient(config.Config{
				Credentials: config.Credentials{AccessKey: "ak_upstream", SecretKey: "sk_upstream"},
				Host:        fakeUpstream.URL,
				Region:      "cn-north-1",
			}, upstream.Options{})
			if err != nil {
				t.Fatalf("NewClient: %v", err)
			}
			auditSvc, _, _, _ := newTestAuditService(t, nil, nil, nil)
			h := NewGetResultHandler(c, auditSvc, nil).Routes()
			req := httptest.NewRequest(http.MethodPost, "/v1/get-result", bytes.NewReader([]byte(`{"task_id":"task_123"}`)))
			req.Header.Set("Content-Type", "application/json")
			req = req.WithContext(context.WithValue(req.Context(), sigv4.ContextAPIKeyID, "k1"))
			rec := httptest.NewRecorder()

			h.ServeHTTP(rec, req)

			if rec.Code != tt.upstreamStatus {
				t.Fatalf("expected status %d, got %d", tt.upstreamStatus, rec.Code)
			}
			if !bytes.Equal(rec.Body.Bytes(), tt.upstreamBody) {
				t.Fatalf("expected body passthrough: %q, got %q", string(tt.upstreamBody), rec.Body.String())
			}
			if got := rec.Header().Get("Content-Type"); got != tt.upstreamContentType {
				t.Fatalf("expected content-type passthrough %q, got %q", tt.upstreamContentType, got)
			}
		})
	}
}
