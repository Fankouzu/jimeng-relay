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

	"github.com/jimeng-relay/client/internal/config"
)

func TestIntegrationSubmitAndQueryMinimal(t *testing.T) {
	ak := strings.TrimSpace(os.Getenv(config.EnvAccessKey))
	sk := strings.TrimSpace(os.Getenv(config.EnvSecretKey))
	if ak == "" || sk == "" {
		t.Skipf("integration test requires %s and %s", config.EnvAccessKey, config.EnvSecretKey)
	}

	cfg, err := config.Load(config.Options{})
	require.NoError(t, err)

	c, err := NewClient(cfg)
	require.NoError(t, err)
	require.NotNil(t, c)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// 1. Submit task
	submitResp, err := c.SubmitTask(ctx, SubmitRequest{
		Prompt:      "integration test: a minimal geometric shape on white background",
		Width:       512,
		Height:      512,
		Scale:       1,
		ForceSingle: true,
	})
	require.NoError(t, err)
	require.NotNil(t, submitResp)
	require.NotEmpty(t, strings.TrimSpace(submitResp.TaskID))

	// 2. Query task status (minimal link)
	// We only check if the query is successful and returns a status.
	// We don't wait for it to be 'done' to avoid instability caused by queuing.
	queryResp, err := c.GetResult(ctx, GetResultRequest{TaskID: submitResp.TaskID})
	require.NoError(t, err)
	require.NotNil(t, queryResp)
	require.NotEmpty(t, queryResp.Status)
}
