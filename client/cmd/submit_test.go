package cmd

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"
)

func TestProcessImageURLs_MixedRemoteAndLocal(t *testing.T) {
	tmpDir := t.TempDir()
	localPath := filepath.Join(tmpDir, "input.png")
	content := []byte("test-image-bytes")
	if err := os.WriteFile(localPath, content, 0o600); err != nil {
		t.Fatalf("write temp image: %v", err)
	}

	remoteURLs, base64Data, err := processImageURLs([]string{
		"https://example.com/a.png",
		localPath,
	})
	if err != nil {
		t.Fatalf("processImageURLs returned error: %v", err)
	}

	if len(remoteURLs) != 1 || remoteURLs[0] != "https://example.com/a.png" {
		t.Fatalf("unexpected remoteURLs: %#v", remoteURLs)
	}
	if len(base64Data) != 1 {
		t.Fatalf("unexpected base64Data length: %d", len(base64Data))
	}

	decoded, err := base64.StdEncoding.DecodeString(base64Data[0])
	if err != nil {
		t.Fatalf("decode base64: %v", err)
	}
	if string(decoded) != string(content) {
		t.Fatalf("decoded payload mismatch: got=%q want=%q", string(decoded), string(content))
	}
}

func TestProcessImageURLs_ExistingRelativePathWithoutPrefix(t *testing.T) {
	tmpDir := t.TempDir()
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	defer func() {
		if err := os.Chdir(oldWd); err != nil {
			t.Errorf("failed to restore working directory: %v", err)
		}
	}()

	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir temp dir: %v", err)
	}

	relPath := "relative-input.png"
	content := []byte("relative-bytes")
	if err := os.WriteFile(relPath, content, 0o600); err != nil {
		t.Fatalf("write relative file: %v", err)
	}

	remoteURLs, base64Data, err := processImageURLs([]string{relPath})
	if err != nil {
		t.Fatalf("processImageURLs returned error: %v", err)
	}
	if len(remoteURLs) != 0 {
		t.Fatalf("unexpected remoteURLs: %#v", remoteURLs)
	}
	if len(base64Data) != 1 {
		t.Fatalf("unexpected base64Data length: %d", len(base64Data))
	}
}
