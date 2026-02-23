package config

import (
	"os"
	"testing"
)

func TestLoad(t *testing.T) {
	// Helper to clear environment variables
	clearEnv := func() {
		os.Unsetenv(EnvAccessKey)
		os.Unsetenv(EnvSecretKey)
		os.Unsetenv(EnvRegion)
		os.Unsetenv(EnvHost)
		os.Unsetenv(EnvTimeout)
		os.Unsetenv(EnvServerPort)
		os.Unsetenv(EnvDatabaseType)
		os.Unsetenv(EnvDatabaseURL)
		os.Unsetenv(EnvAPIKeyEncryptionKey)
	}

	t.Run("DefaultValues", func(t *testing.T) {
		clearEnv()
		os.Setenv(EnvAccessKey, "test-ak")
		os.Setenv(EnvSecretKey, "test-sk")
		os.Setenv(EnvAPIKeyEncryptionKey, "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY=")
		defer clearEnv()

		cfg, err := Load(Options{})
		if err != nil {
			t.Fatalf("Load failed: %v", err)
		}

		if cfg.Region != DefaultRegion {
			t.Errorf("expected region %s, got %s", DefaultRegion, cfg.Region)
		}
		if cfg.Host != DefaultHost {
			t.Errorf("expected host %s, got %s", DefaultHost, cfg.Host)
		}
		if cfg.Timeout != DefaultTimeout {
			t.Errorf("expected timeout %v, got %v", DefaultTimeout, cfg.Timeout)
		}
		if cfg.ServerPort != DefaultServerPort {
			t.Errorf("expected server port %s, got %s", DefaultServerPort, cfg.ServerPort)
		}
		if cfg.DatabaseType != DefaultDatabaseType {
			t.Errorf("expected database type %s, got %s", DefaultDatabaseType, cfg.DatabaseType)
		}
		if cfg.DatabaseURL != DefaultDatabaseURL {
			t.Errorf("expected database URL %s, got %s", DefaultDatabaseURL, cfg.DatabaseURL)
		}
	})

	t.Run("EnvOverrides", func(t *testing.T) {
		clearEnv()
		os.Setenv(EnvAccessKey, "env-ak")
		os.Setenv(EnvSecretKey, "env-sk")
		os.Setenv(EnvRegion, "env-region")
		os.Setenv(EnvServerPort, "9090")
		os.Setenv(EnvAPIKeyEncryptionKey, "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY=")
		defer clearEnv()

		cfg, err := Load(Options{})
		if err != nil {
			t.Fatalf("Load failed: %v", err)
		}

		if cfg.Credentials.AccessKey != "env-ak" {
			t.Errorf("expected AK env-ak, got %s", cfg.Credentials.AccessKey)
		}
		if cfg.Region != "env-region" {
			t.Errorf("expected region env-region, got %s", cfg.Region)
		}
		if cfg.ServerPort != "9090" {
			t.Errorf("expected server port 9090, got %s", cfg.ServerPort)
		}
	})

	t.Run("OptionsOverrides", func(t *testing.T) {
		clearEnv()
		os.Setenv(EnvAccessKey, "env-ak")
		os.Setenv(EnvSecretKey, "env-sk")
		os.Setenv(EnvAPIKeyEncryptionKey, "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY=")
		defer clearEnv()

		ak := "opt-ak"
		port := "7070"
		cfg, err := Load(Options{
			AccessKey:  &ak,
			ServerPort: &port,
		})
		if err != nil {
			t.Fatalf("Load failed: %v", err)
		}

		if cfg.Credentials.AccessKey != "opt-ak" {
			t.Errorf("expected AK opt-ak, got %s", cfg.Credentials.AccessKey)
		}
		if cfg.ServerPort != "7070" {
			t.Errorf("expected server port 7070, got %s", cfg.ServerPort)
		}
	})

	t.Run("EnvFileLoading", func(t *testing.T) {
		clearEnv()
		defer clearEnv()

		envContent := `
VOLC_ACCESSKEY=file-ak
VOLC_SECRETKEY=file-sk
SERVER_PORT=6060
API_KEY_ENCRYPTION_KEY=MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY=
`
		tmpFile := "test.env"
		if err := os.WriteFile(tmpFile, []byte(envContent), 0644); err != nil {
			t.Fatalf("failed to create test env file: %v", err)
		}
		defer os.Remove(tmpFile)

		cfg, err := Load(Options{
			ConfigFile: &tmpFile,
		})
		if err != nil {
			t.Fatalf("Load failed: %v", err)
		}

		if cfg.Credentials.AccessKey != "file-ak" {
			t.Errorf("expected AK file-ak, got %s", cfg.Credentials.AccessKey)
		}
		if cfg.ServerPort != "6060" {
			t.Errorf("expected server port 6060, got %s", cfg.ServerPort)
		}
	})

	t.Run("InvalidTimeout", func(t *testing.T) {
		clearEnv()
		os.Setenv(EnvAccessKey, "ak")
		os.Setenv(EnvSecretKey, "sk")
		os.Setenv(EnvAPIKeyEncryptionKey, "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY=")
		os.Setenv(EnvTimeout, "invalid")
		defer clearEnv()

		_, err := Load(Options{})
		if err == nil {
			t.Error("expected error for invalid timeout, got nil")
		}
	})

	t.Run("MissingCredentials", func(t *testing.T) {
		clearEnv()
		defer clearEnv()

		_, err := Load(Options{})
		if err == nil {
			t.Error("expected error for missing credentials, got nil")
		}
	})
}

func TestRedactAccessKey(t *testing.T) {
	tests := []struct {
		ak       string
		expected string
	}{
		{"", ""},
		{"AK", "AK..."},
		{"AKID", "AKID..."},
		{"AKID123", "AKID..."},
	}

	for _, tt := range tests {
		got := redactAccessKey(tt.ak)
		if got != tt.expected {
			t.Errorf("redactAccessKey(%q) = %q, want %q", tt.ak, got, tt.expected)
		}
	}
}
