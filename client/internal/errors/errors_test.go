package errors

import (
	"errors"
	"testing"
)

func TestErrorClassification(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected Code
	}{
		{
			name:     "AuthFailed",
			err:      New(ErrAuthFailed, "auth failed", nil),
			expected: ErrAuthFailed,
		},
		{
			name:     "RateLimited",
			err:      New(ErrRateLimited, "rate limited", nil),
			expected: ErrRateLimited,
		},
		{
			name:     "Timeout",
			err:      New(ErrTimeout, "timeout", nil),
			expected: ErrTimeout,
		},
		{
			name:     "BusinessFailed",
			err:      New(ErrBusinessFailed, "business failed", nil),
			expected: ErrBusinessFailed,
		},
		{
			name:     "DecodeFailed",
			err:      New(ErrDecodeFailed, "decode failed", nil),
			expected: ErrDecodeFailed,
		},
		{
			name:     "ValidationFailed",
			err:      New(ErrValidationFailed, "validation failed", nil),
			expected: ErrValidationFailed,
		},
		{
			name:     "UnknownError",
			err:      errors.New("standard error"),
			expected: ErrUnknown,
		},
		{
			name:     "NilError",
			err:      nil,
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := GetCode(tt.err); got != tt.expected {
				t.Errorf("GetCode() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestErrorFormatting(t *testing.T) {
	err := New(ErrAuthFailed, "invalid credentials", errors.New("invalid token"))
	expected := "[AUTH_FAILED] invalid credentials: invalid token"
	if err.Error() != expected {
		t.Errorf("Error() = %v, want %v", err.Error(), expected)
	}

	errNoWrap := New(ErrTimeout, "request timeout", nil)
	expectedNoWrap := "[TIMEOUT] request timeout"
	if errNoWrap.Error() != expectedNoWrap {
		t.Errorf("Error() = %v, want %v", errNoWrap.Error(), expectedNoWrap)
	}
}
