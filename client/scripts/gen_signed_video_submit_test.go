package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func runScript(t *testing.T, envs map[string]string) string {
	t.Helper()
	cmd := exec.Command("go", "run", "gen_signed_video_submit.go")
	cmd.Env = os.Environ()
	for k, v := range envs {
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
	}
	// Set required envs if not provided
	required := map[string]string{
		"VOLC_ACCESSKEY": "test-ak",
		"VOLC_SECRETKEY": "test-sk",
	}
	for k, v := range required {
		found := false
		for _, e := range cmd.Env {
			if strings.HasPrefix(e, k+"=") {
				found = true
				break
			}
		}
		if !found {
			cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
		}
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Failed to run script: %v\nOutput: %s", err, string(output))
	}

	var res struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal(output, &res); err != nil {
		t.Fatalf("Failed to parse output: %v\nOutput: %s", err, string(output))
	}
	return res.URL
}

func TestBuildURL_UsesConfiguredScheme(t *testing.T) {
	url := runScript(t, map[string]string{
		"VOLC_HOST":   "localhost:8080",
		"VOLC_SCHEME": "http",
	})

	if !strings.HasPrefix(url, "http://") {
		t.Errorf("Expected URL to start with http:// when VOLC_SCHEME=http, but got %s", url)
	}
}

func TestBuildURL_HostTrailingSlash(t *testing.T) {
	url := runScript(t, map[string]string{
		"VOLC_HOST": "example.com/",
	})

	if strings.Contains(url, "com//?") {
		t.Errorf("URL contains double slash after host: %s", url)
	}
}

func TestBuildURL_HostWithSchemePrefix(t *testing.T) {
	url := runScript(t, map[string]string{
		"VOLC_HOST": "https://example.com",
	})

	// Should not have double scheme like https://https://...
	if strings.HasPrefix(url, "https://https://") {
		t.Errorf("URL has double scheme prefix: %s", url)
	}
}
