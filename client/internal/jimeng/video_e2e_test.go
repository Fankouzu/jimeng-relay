package jimeng

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/jimeng-relay/client/internal/api"
	internalerrors "github.com/jimeng-relay/client/internal/errors"
	"github.com/stretchr/testify/require"
)

func TestVideoFlowE2E_SubmitWaitDownload_Success(t *testing.T) {
	t.Parallel()

	const (
		taskID   = "task-video-e2e-success"
		videoURL = "https://example.com/e2e-success.mp4"
	)
	expectedContent := []byte("e2e-video-content-success")
	tmpDir := t.TempDir()

	orchestrator := &DefaultVideoFlowOrchestrator{
		submitVideoTaskFn: func(context.Context, VideoSubmitRequest) (*VideoSubmitResponse, error) {
			return &VideoSubmitResponse{TaskID: taskID}, nil
		},
		videoWaitFn: func(context.Context, string, VideoPreset, WaitOptions) (*VideoWaitResult, error) {
			return &VideoWaitResult{Status: VideoStatusDone, VideoURL: videoURL, PollCount: 3}, nil
		},
		downloadVideoFn: func(_ context.Context, gotTaskID string, gotVideoURL string, opts FlowOptions) (string, error) {
			require.Equal(t, taskID, gotTaskID)
			require.Equal(t, videoURL, gotVideoURL)
			require.Equal(t, tmpDir, opts.DownloadDir)

			outPath := filepath.Join(opts.DownloadDir, gotTaskID+".mp4")
			if err := os.WriteFile(outPath, expectedContent, 0o644); err != nil {
				return "", err
			}
			return outPath, nil
		},
	}

	res, err := orchestrator.SubmitAndWait(
		context.Background(),
		api.VideoPresetT2V720,
		&VideoSubmitRequest{Prompt: "e2e success"},
		VideoFlowConfig{WaitEnabled: true, DownloadDir: tmpDir, Overwrite: true},
	)

	require.NoError(t, err)
	require.NotNil(t, res)
	require.Equal(t, taskID, res.TaskID)
	require.Equal(t, string(VideoStatusDone), res.Status)
	require.Equal(t, videoURL, res.VideoURL)
	require.NoError(t, res.Error)
	require.FileExists(t, res.DownloadPath)

	got, readErr := os.ReadFile(res.DownloadPath)
	require.NoError(t, readErr)
	require.Equal(t, expectedContent, got)
}

func TestVideoFlowE2E_SubmitWait_Timeout(t *testing.T) {
	t.Parallel()

	const taskID = "task-video-e2e-timeout"
	timeoutErr := internalerrors.New(
		internalerrors.ErrTimeout,
		fmt.Sprintf("task_id=%s wait timeout after 2s (status=generating, polls=2)", taskID),
		context.DeadlineExceeded,
	)

	orchestrator := &DefaultVideoFlowOrchestrator{
		submitVideoTaskFn: func(context.Context, VideoSubmitRequest) (*VideoSubmitResponse, error) {
			return &VideoSubmitResponse{TaskID: taskID}, nil
		},
		videoWaitFn: func(context.Context, string, VideoPreset, WaitOptions) (*VideoWaitResult, error) {
			return nil, timeoutErr
		},
		downloadVideoFn: func(context.Context, string, string, FlowOptions) (string, error) {
			t.Fatalf("download should not be called for timeout path")
			return "", nil
		},
	}

	res, err := orchestrator.SubmitAndWait(
		context.Background(),
		api.VideoPresetT2V720,
		&VideoSubmitRequest{Prompt: "e2e timeout"},
		VideoFlowConfig{WaitEnabled: true, DownloadDir: t.TempDir()},
	)

	require.NoError(t, err)
	require.NotNil(t, res)
	require.Equal(t, taskID, res.TaskID)
	require.Equal(t, "generating", res.Status)
	require.Empty(t, res.VideoURL)
	require.Empty(t, res.DownloadPath)
	require.Error(t, res.Error)
	require.ErrorContains(t, res.Error, taskID)
	require.ErrorContains(t, res.Error, "timeout")
}

func TestVideoFlowE2E_SubmitWaitDownload_DownloadFailure(t *testing.T) {
	t.Parallel()

	const (
		taskID   = "task-video-e2e-download-failure"
		videoURL = "https://example.com/e2e-download-failure.mp4"
	)
	downloadErr := errors.New("mock download write failed")

	orchestrator := &DefaultVideoFlowOrchestrator{
		submitVideoTaskFn: func(context.Context, VideoSubmitRequest) (*VideoSubmitResponse, error) {
			return &VideoSubmitResponse{TaskID: taskID}, nil
		},
		videoWaitFn: func(context.Context, string, VideoPreset, WaitOptions) (*VideoWaitResult, error) {
			return &VideoWaitResult{Status: VideoStatusDone, VideoURL: videoURL, PollCount: 2}, nil
		},
		downloadVideoFn: func(context.Context, string, string, FlowOptions) (string, error) {
			return "", downloadErr
		},
	}

	res, err := orchestrator.SubmitAndWait(
		context.Background(),
		api.VideoPresetT2V720,
		&VideoSubmitRequest{Prompt: "e2e download failure"},
		VideoFlowConfig{WaitEnabled: true, DownloadDir: t.TempDir(), Overwrite: true},
	)

	require.Error(t, err)
	require.NotNil(t, res)
	require.Equal(t, taskID, res.TaskID)
	require.Equal(t, string(VideoStatusDone), res.Status)
	require.Equal(t, videoURL, res.VideoURL)
	require.Empty(t, res.DownloadPath)
	require.Error(t, res.Error)
	require.Equal(t, err.Error(), res.Error.Error())
	require.ErrorContains(t, err, taskID)
	require.ErrorContains(t, err, "download video failed")
	require.ErrorIs(t, err, downloadErr)
}
