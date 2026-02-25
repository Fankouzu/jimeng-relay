package relay

import (
	"errors"
	"net/http"
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
