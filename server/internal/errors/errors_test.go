package errors

import (
	"errors"
	"testing"
)

func TestError(t *testing.T) {
	innerErr := errors.New("inner error")
	err := New(ErrAuthFailed, "auth failed", innerErr)

	if err.Code != ErrAuthFailed {
		t.Errorf("expected code %s, got %s", ErrAuthFailed, err.Code)
	}

	if err.Message != "auth failed" {
		t.Errorf("expected message %s, got %s", "auth failed", err.Message)
	}

	if !errors.Is(err, innerErr) {
		t.Errorf("expected error to wrap inner error")
	}

	expectedStr := "[AUTH_FAILED] auth failed: inner error"
	if err.Error() != expectedStr {
		t.Errorf("expected error string %s, got %s", expectedStr, err.Error())
	}
}

func TestGetCode(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected Code
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: "",
		},
		{
			name:     "custom error",
			err:      New(ErrRateLimited, "too many requests", nil),
			expected: ErrRateLimited,
		},
		{
			name:     "standard error",
			err:      errors.New("standard error"),
			expected: ErrUnknown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code := GetCode(tt.err)
			if code != tt.expected {
				t.Errorf("expected code %s, got %s", tt.expected, code)
			}
		})
	}
}
