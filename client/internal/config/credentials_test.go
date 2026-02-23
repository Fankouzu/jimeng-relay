package config

import (
	"strings"
	"testing"
)

func TestCredentialsMissing(t *testing.T) {
	t.Setenv("VOLC_ACCESSKEY", "")
	t.Setenv("VOLC_SECRETKEY", "")

	_, err := LoadCredentials(CredentialsOptions{})
	if err == nil {
		t.Fatalf("LoadCredentials() expected error, got nil")
	}

	msg := err.Error()
	if !strings.Contains(msg, "VOLC_ACCESSKEY") || !strings.Contains(msg, "VOLC_SECRETKEY") {
		t.Fatalf("error message should mention missing env vars, got: %q", msg)
	}
}
