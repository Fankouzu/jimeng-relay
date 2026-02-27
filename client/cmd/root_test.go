package cmd

import (
	"bytes"
	"os"
	"testing"

	"github.com/jimeng-relay/client/internal/config"
)

func TestRootHelp(t *testing.T) {
	rootCmd := RootCmd()
	b := bytes.NewBufferString("")
	rootCmd.SetOut(b)
	rootCmd.SetArgs([]string{"--help"})

	err := rootCmd.Execute()
	if err != nil {
		t.Fatal(err)
	}

	out := b.String()
	subcommands := []string{"submit", "query", "wait", "download"}
	for _, sub := range subcommands {
		if !contains(out, sub) {
			t.Errorf("expected help output to contain %s", sub)
		}
	}
}

func contains(s, substr string) bool {
	return bytes.Contains([]byte(s), []byte(substr))
}

func TestRootFlags_SchemeExists(t *testing.T) {
	rootCmd := RootCmd()
	schemeFlag := rootCmd.PersistentFlags().Lookup("scheme")
	if schemeFlag == nil {
		t.Fatal("expected --scheme flag to be registered")
	}
}

func TestRootFlags_SchemeOverride(t *testing.T) {
	os.Setenv(config.EnvAccessKey, "test-ak")
	defer os.Unsetenv(config.EnvAccessKey)
	os.Setenv(config.EnvSecretKey, "test-sk")
	defer os.Unsetenv(config.EnvSecretKey)
	os.Setenv(config.EnvScheme, "https")
	defer os.Unsetenv(config.EnvScheme)

	rootCmd := RootCmd()
	// We need to parse flags to trigger flagChanged logic
	err := rootCmd.PersistentFlags().Parse([]string{"--scheme", "http"})
	if err != nil {
		t.Fatalf("failed to parse flags: %v", err)
	}

	cfg, err := loadConfigFromRootFlags(rootCmd)
	if err != nil {
		t.Fatalf("loadConfigFromRootFlags failed: %v", err)
	}

	if cfg.Scheme != "http" {
		t.Errorf("expected scheme 'http' from flag to override env, got %q", cfg.Scheme)
	}
}

func TestRootFlags_HostFromEnv(t *testing.T) {
	os.Setenv(config.EnvAccessKey, "test-ak")
	defer os.Unsetenv(config.EnvAccessKey)
	os.Setenv(config.EnvSecretKey, "test-sk")
	defer os.Unsetenv(config.EnvSecretKey)
	os.Setenv(config.EnvHost, "my-host.com")
	defer os.Unsetenv(config.EnvHost)

	rootCmd := RootCmd()
	_ = rootCmd.PersistentFlags().Parse([]string{})

	cfg, err := loadConfigFromRootFlags(rootCmd)
	if err != nil {
		t.Fatalf("loadConfigFromRootFlags failed: %v", err)
	}

	if cfg.Host != "my-host.com" {
		t.Errorf("expected host 'my-host.com' from env, got %q", cfg.Host)
	}
}
