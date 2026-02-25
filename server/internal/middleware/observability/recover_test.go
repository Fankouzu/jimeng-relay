package observability

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRecoverMiddleware_RecoversPanic(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(testingWriter{t: t}, nil))
	mw := RecoverMiddleware(logger)

	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("boom")
	}))

	req := httptest.NewRequest(http.MethodGet, "/panic", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("content-type = %q, want application/json", ct)
	}
	if !strings.Contains(rec.Body.String(), "INTERNAL_ERROR") {
		t.Fatalf("body = %q, want INTERNAL_ERROR", rec.Body.String())
	}
}

func TestRecoverMiddleware_PassThrough(t *testing.T) {
	mw := RecoverMiddleware(nil)

	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/ok", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}
}

type testingWriter struct {
	t *testing.T
}

func (w testingWriter) Write(p []byte) (int, error) {
	w.t.Logf("log: %s", strings.TrimSpace(string(p)))
	return len(p), nil
}
