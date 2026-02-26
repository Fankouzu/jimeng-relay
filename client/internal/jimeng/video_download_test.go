package jimeng

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"


	"github.com/stretchr/testify/require"
)

func TestVideoDownload_Success(t *testing.T) {
	// Setup mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("fake video content"))
	}))
	defer server.Close()

	// Setup temp dir
	tmpDir, err := os.MkdirTemp("", "video-download-test")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	client := &Client{}
	taskID := "test-task-123"
	videoURL := server.URL + "/video.mp4"

	filePath, err := client.DownloadVideo(context.Background(), taskID, videoURL, FlowOptions{
		DownloadDir: tmpDir,
	})

	require.NoError(t, err)
	require.FileExists(t, filePath)
	require.Contains(t, filePath, "test-task-123-video.mp4")

	content, err := os.ReadFile(filePath)
	require.NoError(t, err)
	require.Equal(t, "fake video content", string(content))
}

func TestVideoDownload_Overwrite(t *testing.T) {
	// Setup temp dir
	tmpDir, err := os.MkdirTemp("", "video-download-overwrite-test")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	taskID := "test-task-overwrite"
	fileName := "test-task-overwrite-video.mp4"
	filePath := filepath.Join(tmpDir, fileName)

	// Create existing file
	err = os.WriteFile(filePath, []byte("old content"), 0644)
	require.NoError(t, err)

	// Setup mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("new content"))
	}))
	defer server.Close()

	client := &Client{}
	videoURL := server.URL + "/video.mp4"

	// Test without overwrite
	_, err = client.DownloadVideo(context.Background(), taskID, videoURL, FlowOptions{
		DownloadDir: tmpDir,
		Overwrite:   false,
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "file already exists")

	// Test with overwrite
	filePath, err = client.DownloadVideo(context.Background(), taskID, videoURL, FlowOptions{
		DownloadDir: tmpDir,
		Overwrite:   true,
	})
	require.NoError(t, err)
	content, err := os.ReadFile(filePath)
	require.NoError(t, err)
	require.Equal(t, "new content", string(content))
}

func TestVideoDownload_ExpiredURL(t *testing.T) {
	// Setup mock server for 403 Forbidden (expired)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()

	tmpDir, err := os.MkdirTemp("", "video-download-expired-test")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	client := &Client{}
	taskID := "test-task-expired"
	videoURL := server.URL + "/expired.mp4"

	_, err = client.DownloadVideo(context.Background(), taskID, videoURL, FlowOptions{
		DownloadDir: tmpDir,
	})

	require.Error(t, err)
	require.Contains(t, err.Error(), "URL might be expired or invalid")
}
