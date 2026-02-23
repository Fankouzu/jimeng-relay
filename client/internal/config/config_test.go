package config

import (
	"testing"
	"time"
)

func TestConfigLoadFromEnv(t *testing.T) {
	t.Setenv("VOLC_ACCESSKEY", "AK123456")
	t.Setenv("VOLC_SECRETKEY", "SK-should-not-appear")
	t.Setenv("VOLC_REGION", "cn-test-1")
	t.Setenv("VOLC_HOST", "example.com")
	t.Setenv("VOLC_TIMEOUT", "45s")

	cfg, err := Load(Options{})
	if err != nil {
		t.Fatalf("Load() unexpected error: %v", err)
	}
	if cfg.Credentials.AccessKey != "AK123456" {
		t.Fatalf("AccessKey mismatch: got %q", cfg.Credentials.AccessKey)
	}
	if cfg.Credentials.SecretKey != "SK-should-not-appear" {
		t.Fatalf("SecretKey mismatch: got %q", cfg.Credentials.SecretKey)
	}
	if cfg.Region != "cn-test-1" {
		t.Fatalf("Region mismatch: got %q", cfg.Region)
	}
	if cfg.Host != "example.com" {
		t.Fatalf("Host mismatch: got %q", cfg.Host)
	}
	if cfg.Timeout != 45*time.Second {
		t.Fatalf("Timeout mismatch: got %s", cfg.Timeout)
	}
}
