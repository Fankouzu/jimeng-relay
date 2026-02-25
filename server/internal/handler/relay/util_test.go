package relay

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	internalerrors "github.com/jimeng-relay/server/internal/errors"
)

func TestErrorToStatus(t *testing.T) {
	tests := []struct {
		name   string
		err    error
		expect int
	}{
		{
			name:   "AuthFailed",
			err:    internalerrors.New(internalerrors.ErrAuthFailed, "auth failed", nil),
			expect: http.StatusUnauthorized,
		},
		{
			name:   "KeyRevoked",
			err:    internalerrors.New(internalerrors.ErrKeyRevoked, "key revoked", nil),
			expect: http.StatusUnauthorized,
		},
		{
			name:   "KeyExpired",
			err:    internalerrors.New(internalerrors.ErrKeyExpired, "key expired", nil),
			expect: http.StatusUnauthorized,
		},
		{
			name:   "InvalidSignature",
			err:    internalerrors.New(internalerrors.ErrInvalidSignature, "invalid signature", nil),
			expect: http.StatusUnauthorized,
		},
		{
			name:   "RateLimited",
			err:    internalerrors.New(internalerrors.ErrRateLimited, "rate limited", nil),
			expect: http.StatusTooManyRequests,
		},
		{
			name:   "ValidationFailed",
			err:    internalerrors.New(internalerrors.ErrValidationFailed, "validation failed", nil),
			expect: http.StatusBadRequest,
		},
		{
			name:   "UpstreamFailed",
			err:    internalerrors.New(internalerrors.ErrUpstreamFailed, "upstream failed", nil),
			expect: http.StatusBadGateway,
		},
		{
			name:   "InternalError",
			err:    internalerrors.New(internalerrors.ErrInternalError, "internal error", nil),
			expect: http.StatusInternalServerError,
		},
		{
			name:   "DatabaseError",
			err:    internalerrors.New(internalerrors.ErrDatabaseError, "database error", nil),
			expect: http.StatusInternalServerError,
		},
		{
			name:   "AuditFailed",
			err:    internalerrors.New(internalerrors.ErrAuditFailed, "audit failed", nil),
			expect: http.StatusInternalServerError,
		},
		{
			name:   "UnknownError",
			err:    errors.New("unknown"),
			expect: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ErrorToStatus(tt.err)
			if got != tt.expect {
				t.Errorf("ErrorToStatus() = %v, want %v", got, tt.expect)
			}
		})
	}
}

func TestReadRequestBodyLimited_AllowsBodyUpToLimit(t *testing.T) {
	body := strings.Repeat("a", int(maxDownstreamBodyBytes))
	req := httptest.NewRequest(http.MethodPost, "/v1/submit", strings.NewReader(body))

	got, err := readRequestBodyLimited(req)
	if err != nil {
		t.Fatalf("readRequestBodyLimited() error = %v", err)
	}
	if len(got) != len(body) {
		t.Fatalf("readRequestBodyLimited() len = %d, want %d", len(got), len(body))
	}
}

func TestReadRequestBodyLimited_RejectsBodyAboveLimit(t *testing.T) {
	body := strings.Repeat("a", int(maxDownstreamBodyBytes)+1)
	req := httptest.NewRequest(http.MethodPost, "/v1/submit", strings.NewReader(body))

	_, err := readRequestBodyLimited(req)
	if err == nil {
		t.Fatalf("readRequestBodyLimited() expected error, got nil")
	}
	if !strings.Contains(err.Error(), "request body too large") {
		t.Fatalf("readRequestBodyLimited() error = %q, want contains %q", err.Error(), "request body too large")
	}
}

func TestReadRequestBodyLimited_NilRequest(t *testing.T) {
	got, err := readRequestBodyLimited(nil)
	if err != nil {
		t.Fatalf("readRequestBodyLimited(nil) error = %v", err)
	}
	if got != nil {
		t.Fatalf("readRequestBodyLimited(nil) = %v, want nil", got)
	}
}

func TestReadRequestBodyLimited_ReaderError(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/v1/submit", io.NopCloser(errReader{}))

	_, err := readRequestBodyLimited(req)
	if err == nil {
		t.Fatalf("readRequestBodyLimited() expected error, got nil")
	}
}

type errReader struct{}

func (errReader) Read(_ []byte) (int, error) {
	return 0, errors.New("read failed")
}
