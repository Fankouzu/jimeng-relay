package config

import (
	"testing"
	"os"
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
func TestLoad_HostSchemePriority(t *testing.T) {
	// Setup .env file
	t.Setenv("VOLC_ACCESSKEY", "dummy")
	t.Setenv("VOLC_SECRETKEY", "dummy")
	envFile := ".env.test"
	content := "VOLC_HOST=env-file.com\nVOLC_SCHEME=http"
	if err := os.WriteFile(envFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to create test .env file: %v", err)
	}
	defer os.Remove(envFile)

	t.Run("Default", func(t *testing.T) {
		os.Clearenv()
		t.Setenv("VOLC_ACCESSKEY", "dummy")
		t.Setenv("VOLC_SECRETKEY", "dummy")
		emptyEnv := ".non-existent"
		cfg, err := Load(Options{ConfigFile: &emptyEnv})
		if err != nil {
			t.Fatalf("Load() unexpected error: %v", err)
		}
		if cfg.Host != DefaultHost {
			t.Errorf("Host mismatch: got %q, want %q", cfg.Host, DefaultHost)
		}
		if cfg.Scheme != DefaultScheme {
			t.Errorf("Scheme mismatch: got %q, want %q", cfg.Scheme, DefaultScheme)
		}
	})

	t.Run(".env > Default", func(t *testing.T) {
		os.Clearenv()
		t.Setenv("VOLC_ACCESSKEY", "dummy")
		t.Setenv("VOLC_SECRETKEY", "dummy")
		cfg, err := Load(Options{ConfigFile: &envFile})
		if err != nil {
			t.Fatalf("Load() unexpected error: %v", err)
		}
		if cfg.Host != "env-file.com" {
			t.Errorf("Host mismatch: got %q, want %q", cfg.Host, "env-file.com")
		}
		if cfg.Scheme != "http" {
			t.Errorf("Scheme mismatch: got %q, want %q", cfg.Scheme, "http")
		}
	})

	t.Run("Env > .env", func(t *testing.T) {
		os.Clearenv()
		t.Setenv("VOLC_ACCESSKEY", "dummy")
		t.Setenv("VOLC_SECRETKEY", "dummy")
		t.Setenv("VOLC_HOST", "env-var.com")
		t.Setenv("VOLC_SCHEME", "https")
		cfg, err := Load(Options{ConfigFile: &envFile})
		if err != nil {
			t.Fatalf("Load() unexpected error: %v", err)
		}
		if cfg.Host != "env-var.com" {
			t.Errorf("Host mismatch: got %q, want %q", cfg.Host, "env-var.com")
		}
		if cfg.Scheme != "https" {
			t.Errorf("Scheme mismatch: got %q, want %q", cfg.Scheme, "https")
		}
	})

	t.Run("Flag > Env", func(t *testing.T) {
		os.Clearenv()
		t.Setenv("VOLC_ACCESSKEY", "dummy")
		t.Setenv("VOLC_SECRETKEY", "dummy")
		t.Setenv("VOLC_HOST", "env-var.com")
		hostFlag := "flag.com"
		cfg, err := Load(Options{Host: &hostFlag, ConfigFile: &envFile})
		if err != nil {
			t.Fatalf("Load() unexpected error: %v", err)
		}
		if cfg.Host != "flag.com" {
			t.Errorf("Host mismatch: got %q, want %q", cfg.Host, "flag.com")
		}
	})
}

func TestLoad_HostNormalization(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"Remove https prefix", "https://api.example.com", "api.example.com"},
		{"Remove http prefix", "http://api.example.com", "api.example.com"},
		{"Remove trailing slash", "api.example.com/", "api.example.com"},
		{"Remove both", "https://api.example.com/", "api.example.com"},
		{"No change", "api.example.com", "api.example.com"},
	}

	t.Setenv("VOLC_ACCESSKEY", "dummy")
	t.Setenv("VOLC_SECRETKEY", "dummy")
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := Load(Options{Host: &tt.input})
			if err != nil {
				t.Fatalf("Load() unexpected error: %v", err)
			}
			if cfg.Host != tt.expected {
				t.Errorf("Host normalization failed: got %q, want %q", cfg.Host, tt.expected)
			}
		})
	}
}

func TestLoad_SchemeValidation(t *testing.T) {
	tests := []struct {
		name   string
		scheme string
		valid  bool
	}{
		{"Valid https", "https", true},
		{"Valid http", "http", true},
		{"Invalid ftp", "ftp", false},
		{"Invalid empty", "", false},
		{"Invalid case", "HTTPS", false}, // Should we allow case-insensitive? Requirement says "只允许 http/https"
	}

	t.Setenv("VOLC_ACCESSKEY", "dummy")
	t.Setenv("VOLC_SECRETKEY", "dummy")
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("VOLC_SCHEME", tt.scheme)
			_, err := Load(Options{})
			if tt.valid && err != nil {
				t.Errorf("Expected valid scheme %q, got error: %v", tt.scheme, err)
			}
			if !tt.valid && err == nil {
				t.Errorf("Expected error for invalid scheme %q, got nil", tt.scheme)
			}
		})
	}
}

func TestLoad_InvalidScheme(t *testing.T) {
	t.Setenv("VOLC_ACCESSKEY", "dummy")
	t.Setenv("VOLC_SECRETKEY", "dummy")
	t.Setenv("VOLC_SCHEME", "invalid")
	_, err := Load(Options{})
	if err == nil {
		t.Fatal("Expected error for invalid scheme, got nil")
	}
}
