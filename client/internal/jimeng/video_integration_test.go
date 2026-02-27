//go:build integration
// +build integration

package jimeng

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/jimeng-relay/client/internal/api"
	"github.com/jimeng-relay/client/internal/config"
)

const (
	envIntegrationRelayHost        = "JIMENG_INTEGRATION_RELAY_HOST"
	envIntegrationRelayScheme      = "JIMENG_INTEGRATION_RELAY_SCHEME"
	envIntegrationRelayTimeout     = "JIMENG_INTEGRATION_RELAY_TIMEOUT"
	envIntegrationVideoI2VImageURL = "JIMENG_INTEGRATION_VIDEO_I2V_IMAGE_URL"
)

func TestIntegrationVideoDirectT2V(t *testing.T) {
	ensureBaseCredentialsOrSkip(t)

	host := strings.TrimSpace(os.Getenv(config.EnvHost))
	if host == "" {
		t.Skipf("integration test requires %s", config.EnvHost)
	}
	scheme := config.DefaultScheme
	timeout := 8 * time.Minute

	cfg, err := config.Load(config.Options{
		Host:    &host,
		Timeout: &timeout,
	})
	require.NoError(t, err)
	cfg.Scheme = scheme

	c, err := NewClient(cfg)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()

	resp, err := c.SubmitVideoTask(ctx, VideoSubmitRequest{
		Preset:      api.VideoPresetT2V720,
		Prompt:      "integration test direct t2v: sunrise over snow mountains",
		Frames:      121,
		AspectRatio: "16:9",
		Seed:        20260226,
	})
	require.NoError(t, err)
	require.NotEmpty(t, strings.TrimSpace(resp.TaskID))

	waitResp, err := c.VideoWait(ctx, resp.TaskID, resp.Preset, WaitOptions{
		Interval: 2 * time.Second,
		Timeout:  7 * time.Minute,
	})
	require.NoError(t, err)
	require.Equal(t, VideoStatusDone, waitResp.Status)
	require.NotEmpty(t, strings.TrimSpace(waitResp.VideoURL))

	filePath, err := c.DownloadVideo(ctx, resp.TaskID, waitResp.VideoURL, FlowOptions{
		DownloadDir: t.TempDir(),
		Overwrite:   true,
	})
	require.NoError(t, err)

	info, err := os.Stat(filePath)
	require.NoError(t, err)
	require.Greater(t, info.Size(), int64(0))
}

func TestIntegrationVideoRelayI2V(t *testing.T) {
	ensureBaseCredentialsOrSkip(t)

	relayHost := strings.TrimSpace(os.Getenv(envIntegrationRelayHost))
	if relayHost == "" {
		t.Skipf("integration test requires %s for relay mode", envIntegrationRelayHost)
	}

	imageURL := strings.TrimSpace(os.Getenv(envIntegrationVideoI2VImageURL))
	if imageURL == "" {
		t.Skipf("integration test requires %s for i2v input image", envIntegrationVideoI2VImageURL)
	}

	relayScheme := strings.TrimSpace(os.Getenv(envIntegrationRelayScheme))
	if relayScheme == "" {
		relayScheme = "http"
	}

	timeout := 8 * time.Minute
	if rawTimeout := strings.TrimSpace(os.Getenv(envIntegrationRelayTimeout)); rawTimeout != "" {
		parsed, err := time.ParseDuration(rawTimeout)
		if err != nil {
			t.Skipf("integration test invalid %s=%q: %v", envIntegrationRelayTimeout, rawTimeout, err)
		}
		timeout = parsed
	}

	cfg, err := config.Load(config.Options{
		Host:    &relayHost,
		Timeout: &timeout,
	})
	require.NoError(t, err)
	cfg.Scheme = relayScheme

	c, err := NewClient(cfg)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	resp, err := c.SubmitVideoTask(ctx, VideoSubmitRequest{
		Preset:    api.VideoPresetI2VFirst,
		Prompt:    "integration test relay i2v: animate a subtle camera move",
		ImageURLs: []string{imageURL},
	})
	require.NoError(t, err)
	require.NotEmpty(t, strings.TrimSpace(resp.TaskID))

	waitTimeout := timeout - time.Minute
	if waitTimeout <= 0 {
		waitTimeout = timeout
	}
	waitResp, err := c.VideoWait(ctx, resp.TaskID, resp.Preset, WaitOptions{
		Interval: 2 * time.Second,
		Timeout:  waitTimeout,
	})
	require.NoError(t, err)
	require.Equal(t, VideoStatusDone, waitResp.Status)
	require.NotEmpty(t, strings.TrimSpace(waitResp.VideoURL))

	filePath, err := c.DownloadVideo(ctx, resp.TaskID, waitResp.VideoURL, FlowOptions{
		DownloadDir: t.TempDir(),
		Overwrite:   true,
	})
	require.NoError(t, err)

	info, err := os.Stat(filePath)
	require.NoError(t, err)
	require.Greater(t, info.Size(), int64(0))
}

func ensureBaseCredentialsOrSkip(t *testing.T) {
	t.Helper()

	ak := strings.TrimSpace(os.Getenv(config.EnvAccessKey))
	sk := strings.TrimSpace(os.Getenv(config.EnvSecretKey))
	if ak == "" || sk == "" {
		t.Skipf("integration test requires %s and %s", config.EnvAccessKey, config.EnvSecretKey)
	}
}
