package jimeng

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jimeng-relay/client/internal/api"
	internalerrors "github.com/jimeng-relay/client/internal/errors"
	"github.com/stretchr/testify/require"
)

func TestVideoFlowConfigValidate(t *testing.T) {
	t.Parallel()

	t.Run("rejects negative timeout", func(t *testing.T) {
		t.Parallel()

		err := (VideoFlowConfig{WaitEnabled: true, WaitTimeout: -1 * time.Second}).Validate()
		require.Error(t, err)
		require.ErrorContains(t, err, "must not be negative")
	})

	t.Run("rejects download without wait", func(t *testing.T) {
		t.Parallel()

		err := (VideoFlowConfig{WaitEnabled: false, DownloadDir: "./out"}).Validate()
		require.Error(t, err)
		require.ErrorContains(t, err, "download requires waiting")
	})

	t.Run("accepts wait with default timeout", func(t *testing.T) {
		t.Parallel()

		err := (VideoFlowConfig{WaitEnabled: true}).Validate()
		require.NoError(t, err)
	})

	t.Run("accepts wait+download", func(t *testing.T) {
		t.Parallel()

		err := (VideoFlowConfig{WaitEnabled: true, DownloadDir: "./out", Overwrite: true}).Validate()
		require.NoError(t, err)
	})
}

type mockVideoFlowOrchestrator struct {
	submitFn        func(ctx context.Context, preset VideoPreset, req *VideoSubmitRequest) (*VideoSubmitResult, error)
	submitAndWaitFn func(ctx context.Context, preset VideoPreset, req *VideoSubmitRequest, cfg VideoFlowConfig) (*VideoFlowResult, error)
}

func (m *mockVideoFlowOrchestrator) Submit(ctx context.Context, preset VideoPreset, req *VideoSubmitRequest) (*VideoSubmitResult, error) {
	if m.submitFn == nil {
		return nil, nil
	}
	return m.submitFn(ctx, preset, req)
}

func (m *mockVideoFlowOrchestrator) SubmitAndWait(ctx context.Context, preset VideoPreset, req *VideoSubmitRequest, cfg VideoFlowConfig) (*VideoFlowResult, error) {
	if m.submitAndWaitFn == nil {
		return nil, nil
	}
	return m.submitAndWaitFn(ctx, preset, req, cfg)
}

func TestVideoFlowOrchestratorContract_Compiles(t *testing.T) {
	t.Parallel()

	var _ VideoFlowOrchestrator = (*mockVideoFlowOrchestrator)(nil)
}

func TestVideoWait_BackwardCompatibility_MethodSignature(t *testing.T) {
	t.Parallel()

	var _ func(*Client, context.Context, string, api.VideoPreset, WaitOptions) (*VideoWaitResult, error) = (*Client).VideoWait
}

func TestDefaultVideoFlowOrchestrator_Submit_Delegates(t *testing.T) {
	t.Parallel()

	called := 0
	o := &DefaultVideoFlowOrchestrator{
		submitVideoTaskFn: func(ctx context.Context, req VideoSubmitRequest) (*VideoSubmitResponse, error) {
			called++
			require.Equal(t, api.VideoPresetT2V720, req.Preset)
			require.Equal(t, "hello", req.Prompt)
			return &VideoSubmitResponse{TaskID: "task-video-1"}, nil
		},
	}

	resp, err := o.Submit(context.Background(), api.VideoPresetT2V720, &VideoSubmitRequest{Prompt: "hello"})
	require.NoError(t, err)
	require.Equal(t, 1, called)
	require.NotNil(t, resp)
	require.Equal(t, "task-video-1", resp.TaskID)
}

func TestDefaultVideoFlowOrchestrator_SubmitAndWait_NoWait_ReturnsAfterSubmit(t *testing.T) {
	t.Parallel()

	o := &DefaultVideoFlowOrchestrator{
		submitVideoTaskFn: func(ctx context.Context, req VideoSubmitRequest) (*VideoSubmitResponse, error) {
			return &VideoSubmitResponse{TaskID: "task-video-2"}, nil
		},
		videoWaitFn: func(context.Context, string, VideoPreset, WaitOptions) (*VideoWaitResult, error) {
			t.Fatalf("videoWaitFn should not be called")
			return nil, nil
		},
		downloadVideoFn: func(context.Context, string, string, FlowOptions) (string, error) {
			t.Fatalf("downloadVideoFn should not be called")
			return "", nil
		},
	}

	res, err := o.SubmitAndWait(context.Background(), api.VideoPresetT2V720, &VideoSubmitRequest{Prompt: "x"}, VideoFlowConfig{WaitEnabled: false})
	require.NoError(t, err)
	require.NotNil(t, res)
	require.Equal(t, "task-video-2", res.TaskID)
	require.Empty(t, res.Status)
	require.Empty(t, res.VideoURL)
	require.Empty(t, res.DownloadPath)
	require.NoError(t, res.Error)
}

func TestDefaultVideoFlowOrchestrator_SubmitAndWait_WaitEnabled_CallsWait(t *testing.T) {
	t.Parallel()

	calledWait := 0
	timeout := 123 * time.Second
	o := &DefaultVideoFlowOrchestrator{
		submitVideoTaskFn: func(ctx context.Context, req VideoSubmitRequest) (*VideoSubmitResponse, error) {
			return &VideoSubmitResponse{TaskID: "task-video-3"}, nil
		},
		videoWaitFn: func(ctx context.Context, taskID string, preset VideoPreset, opts WaitOptions) (*VideoWaitResult, error) {
			calledWait++
			require.Equal(t, "task-video-3", taskID)
			require.Equal(t, api.VideoPresetT2V720, preset)
			require.Equal(t, timeout, opts.Timeout)
			return &VideoWaitResult{Status: VideoStatusDone, VideoURL: "https://example.com/v.mp4", PollCount: 7}, nil
		},
		downloadVideoFn: func(context.Context, string, string, FlowOptions) (string, error) {
			t.Fatalf("downloadVideoFn should not be called")
			return "", nil
		},
	}

	res, err := o.SubmitAndWait(context.Background(), api.VideoPresetT2V720, &VideoSubmitRequest{Prompt: "x"}, VideoFlowConfig{WaitEnabled: true, WaitTimeout: timeout})
	require.NoError(t, err)
	require.Equal(t, 1, calledWait)
	require.NotNil(t, res)
	require.Equal(t, "task-video-3", res.TaskID)
	require.Equal(t, string(VideoStatusDone), res.Status)
	require.Equal(t, "https://example.com/v.mp4", res.VideoURL)
	require.Empty(t, res.DownloadPath)
	require.NoError(t, res.Error)
}

func TestDefaultVideoFlowOrchestrator_SubmitAndWait_Download_WhenDownloadDirProvided(t *testing.T) {
	t.Parallel()

	calledDownload := 0
	o := &DefaultVideoFlowOrchestrator{
		submitVideoTaskFn: func(ctx context.Context, req VideoSubmitRequest) (*VideoSubmitResponse, error) {
			return &VideoSubmitResponse{TaskID: "task-video-4"}, nil
		},
		videoWaitFn: func(ctx context.Context, taskID string, preset VideoPreset, opts WaitOptions) (*VideoWaitResult, error) {
			return &VideoWaitResult{Status: VideoStatusDone, VideoURL: "https://example.com/v2.mp4", PollCount: 1}, nil
		},
		downloadVideoFn: func(ctx context.Context, taskID string, videoURL string, opts FlowOptions) (string, error) {
			calledDownload++
			require.Equal(t, "task-video-4", taskID)
			require.Equal(t, "https://example.com/v2.mp4", videoURL)
			require.Equal(t, "./out", opts.DownloadDir)
			require.True(t, opts.Overwrite)
			return "./out/task-video-4.mp4", nil
		},
	}

	res, err := o.SubmitAndWait(context.Background(), api.VideoPresetT2V720, &VideoSubmitRequest{Prompt: "x"}, VideoFlowConfig{WaitEnabled: true, DownloadDir: " ./out ", Overwrite: true})
	require.NoError(t, err)
	require.Equal(t, 1, calledDownload)
	require.NotNil(t, res)
	require.Equal(t, "./out/task-video-4.mp4", res.DownloadPath)
}

func TestDefaultVideoFlowOrchestrator_SubmitAndWait_WaitBusinessFailed_ReturnsResultWithError(t *testing.T) {
	t.Parallel()

	waitErr := internalerrors.New(internalerrors.ErrBusinessFailed, "video failed", nil)
	o := &DefaultVideoFlowOrchestrator{
		submitVideoTaskFn: func(ctx context.Context, req VideoSubmitRequest) (*VideoSubmitResponse, error) {
			return &VideoSubmitResponse{TaskID: "task-video-5"}, nil
		},
		videoWaitFn: func(context.Context, string, VideoPreset, WaitOptions) (*VideoWaitResult, error) {
			return nil, waitErr
		},
	}

	res, err := o.SubmitAndWait(context.Background(), api.VideoPresetT2V720, &VideoSubmitRequest{Prompt: "x"}, VideoFlowConfig{WaitEnabled: true})
	require.NoError(t, err)
	require.NotNil(t, res)
	require.Equal(t, "task-video-5", res.TaskID)
	require.Equal(t, waitErr, res.Error)
}

func TestDefaultVideoFlowOrchestrator_SubmitAndWait_WaitAuthFailed_ReturnsErrorAndResult(t *testing.T) {
	t.Parallel()

	waitErr := internalerrors.New(internalerrors.ErrAuthFailed, "auth failed", nil)
	o := &DefaultVideoFlowOrchestrator{
		submitVideoTaskFn: func(ctx context.Context, req VideoSubmitRequest) (*VideoSubmitResponse, error) {
			return &VideoSubmitResponse{TaskID: "task-video-6"}, nil
		},
		videoWaitFn: func(context.Context, string, VideoPreset, WaitOptions) (*VideoWaitResult, error) {
			return nil, waitErr
		},
	}

	res, err := o.SubmitAndWait(context.Background(), api.VideoPresetT2V720, &VideoSubmitRequest{Prompt: "x"}, VideoFlowConfig{WaitEnabled: true})
	require.Error(t, err)
	require.NotNil(t, res)
	require.Equal(t, "task-video-6", res.TaskID)
	require.Equal(t, waitErr, res.Error)
}

func TestDefaultVideoFlowOrchestrator_SubmitAndWait_WaitTimeout_ReturnsResultWithStatusAndError(t *testing.T) {
	t.Parallel()

	waitErr := internalerrors.New(
		internalerrors.ErrTimeout,
		"wait timeout after 10s (status=generating, polls=3)",
		context.DeadlineExceeded,
	)

	o := &DefaultVideoFlowOrchestrator{
		submitVideoTaskFn: func(ctx context.Context, req VideoSubmitRequest) (*VideoSubmitResponse, error) {
			return &VideoSubmitResponse{TaskID: "task-video-timeout-1"}, nil
		},
		videoWaitFn: func(context.Context, string, VideoPreset, WaitOptions) (*VideoWaitResult, error) {
			return nil, waitErr
		},
	}

	res, err := o.SubmitAndWait(
		context.Background(),
		api.VideoPresetT2V720,
		&VideoSubmitRequest{Prompt: "x"},
		VideoFlowConfig{WaitEnabled: true, WaitTimeout: 10 * time.Second},
	)
	require.NoError(t, err)
	require.NotNil(t, res)
	require.Equal(t, "task-video-timeout-1", res.TaskID)
	require.Equal(t, "generating", res.Status)
	require.Equal(t, waitErr, res.Error)
}

func TestDefaultVideoFlowOrchestrator_SubmitAndWait_Download_WritesFile(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("video-content-1"))
	}))
	defer server.Close()

	client := &Client{}
	tmpDir := t.TempDir()
	videoURL := server.URL + "/video.mp4"

	o := &DefaultVideoFlowOrchestrator{
		submitVideoTaskFn: func(ctx context.Context, req VideoSubmitRequest) (*VideoSubmitResponse, error) {
			return &VideoSubmitResponse{TaskID: "task-video-dl-1"}, nil
		},
		videoWaitFn: func(context.Context, string, VideoPreset, WaitOptions) (*VideoWaitResult, error) {
			return &VideoWaitResult{Status: VideoStatusDone, VideoURL: videoURL, PollCount: 1}, nil
		},
		downloadVideoFn: client.DownloadVideo,
	}

	res, err := o.SubmitAndWait(
		context.Background(),
		api.VideoPresetT2V720,
		&VideoSubmitRequest{Prompt: "x"},
		VideoFlowConfig{WaitEnabled: true, DownloadDir: tmpDir},
	)
	require.NoError(t, err)
	require.NotNil(t, res)
	require.Equal(t, "task-video-dl-1", res.TaskID)
	require.Equal(t, string(VideoStatusDone), res.Status)
	require.Equal(t, videoURL, res.VideoURL)
	require.NotEmpty(t, res.DownloadPath)
	require.FileExists(t, res.DownloadPath)

	content, err := os.ReadFile(res.DownloadPath)
	require.NoError(t, err)
	require.Equal(t, "video-content-1", string(content))
}

func TestDefaultVideoFlowOrchestrator_SubmitAndWait_Download_HonorsOverwrite(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("video-content-2"))
	}))
	defer server.Close()

	client := &Client{}
	tmpDir := t.TempDir()
	taskID := "task-video-dl-2"
	videoURL := server.URL + "/video.mp4"

	filePath := filepath.Join(tmpDir, sanitizeTaskID(taskID)+"-video.mp4")
	err := os.WriteFile(filePath, []byte("old"), 0o644)
	require.NoError(t, err)

	o := &DefaultVideoFlowOrchestrator{
		submitVideoTaskFn: func(ctx context.Context, req VideoSubmitRequest) (*VideoSubmitResponse, error) {
			return &VideoSubmitResponse{TaskID: taskID}, nil
		},
		videoWaitFn: func(context.Context, string, VideoPreset, WaitOptions) (*VideoWaitResult, error) {
			return &VideoWaitResult{Status: VideoStatusDone, VideoURL: videoURL, PollCount: 1}, nil
		},
		downloadVideoFn: client.DownloadVideo,
	}

	res, err := o.SubmitAndWait(
		context.Background(),
		api.VideoPresetT2V720,
		&VideoSubmitRequest{Prompt: "x"},
		VideoFlowConfig{WaitEnabled: true, DownloadDir: tmpDir, Overwrite: false},
	)
	require.Error(t, err)
	require.NotNil(t, res)
	require.NotNil(t, res.Error)
	require.Equal(t, err.Error(), res.Error.Error())

	content, readErr := os.ReadFile(filePath)
	require.NoError(t, readErr)
	require.Equal(t, "old", string(content))

	res, err = o.SubmitAndWait(
		context.Background(),
		api.VideoPresetT2V720,
		&VideoSubmitRequest{Prompt: "x"},
		VideoFlowConfig{WaitEnabled: true, DownloadDir: tmpDir, Overwrite: true},
	)
	require.NoError(t, err)
	require.NotNil(t, res)
	require.Equal(t, filePath, res.DownloadPath)

	content, readErr = os.ReadFile(filePath)
	require.NoError(t, readErr)
	require.Equal(t, "video-content-2", string(content))
}

func TestDefaultVideoFlowOrchestrator_SubmitAndWait_DownloadError_ReturnsErrorAndResult(t *testing.T) {
	t.Parallel()

	dlErr := errors.New("download failed")

	o := &DefaultVideoFlowOrchestrator{
		submitVideoTaskFn: func(ctx context.Context, req VideoSubmitRequest) (*VideoSubmitResponse, error) {
			return &VideoSubmitResponse{TaskID: "task-video-dl-err"}, nil
		},
		videoWaitFn: func(context.Context, string, VideoPreset, WaitOptions) (*VideoWaitResult, error) {
			return &VideoWaitResult{Status: VideoStatusDone, VideoURL: "https://example.com/v.mp4", PollCount: 1}, nil
		},
		downloadVideoFn: func(context.Context, string, string, FlowOptions) (string, error) {
			return "", dlErr
		},
	}

	res, err := o.SubmitAndWait(
		context.Background(),
		api.VideoPresetT2V720,
		&VideoSubmitRequest{Prompt: "x"},
		VideoFlowConfig{WaitEnabled: true, DownloadDir: t.TempDir(), Overwrite: true},
	)
	require.Error(t, err)
	require.NotNil(t, res)
	require.NotNil(t, res.Error)
	require.Equal(t, err.Error(), res.Error.Error())
	require.ErrorIs(t, err, dlErr)
}
