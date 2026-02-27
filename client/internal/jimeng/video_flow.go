package jimeng

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jimeng-relay/client/internal/api"
	internalerrors "github.com/jimeng-relay/client/internal/errors"
)

// VideoPreset is the video model preset.
//
// It is an alias of client/internal/api.VideoPreset so higher-level orchestration
// code can depend on the jimeng client package without importing api directly.
type VideoPreset = api.VideoPreset

// VideoSubmitResult is the result returned by a video submit operation.
//
// This is currently an alias of VideoSubmitResponse.
type VideoSubmitResult = VideoSubmitResponse

// VideoFlowConfig configures a video task flow with optional waiting and
// optional downloading.
//
// This struct is decoupled from CLI flags so it can be reused by both commands
// and programmatic callers.
type VideoFlowConfig struct {
	// WaitEnabled controls whether SubmitAndWait performs a polling wait.
	WaitEnabled bool

	// WaitTimeout is the max duration to wait when WaitEnabled is true.
	// A zero value means the underlying VideoWait default is used.
	WaitTimeout time.Duration

	// DownloadDir enables download when non-empty.
	// If set, the flow requires waiting until the task is done.
	DownloadDir string

	// Overwrite controls whether an existing output file can be overwritten.
	Overwrite bool
}

// Validate performs basic option validation.
//
// It does not validate filesystem state (e.g. directory existence) because that
// is an implementation concern.
func (cfg VideoFlowConfig) Validate() error {
	if cfg.WaitTimeout < 0 {
		return fmt.Errorf("wait timeout must not be negative")
	}

	if strings.TrimSpace(cfg.DownloadDir) != "" && !cfg.WaitEnabled {
		return fmt.Errorf("download requires waiting: set WaitEnabled=true when DownloadDir is provided")
	}

	return nil
}

// VideoFlowResult is the high-level outcome returned by SubmitAndWait.
//
// Error is intended to capture task-level failures (e.g. terminal status=failed)
// while keeping TaskID available to the caller. Transport/configuration failures
// should be returned as a non-nil error from SubmitAndWait.
type VideoFlowResult struct {
	TaskID       string
	Status       string
	VideoURL     string
	DownloadPath string
	Error        error
}

// VideoFlowOrchestrator defines the orchestration boundary for video flows.
//
// Implementations should reuse existing Client.VideoWait polling semantics and
// Client.DownloadVideo instead of re-implementing them.
type VideoFlowOrchestrator interface {
	// Submit enqueues a video task and returns the submit result.
	Submit(ctx context.Context, preset VideoPreset, req *VideoSubmitRequest) (*VideoSubmitResult, error)

	// SubmitAndWait submits a task and then optionally waits and downloads
	// according to cfg.
	SubmitAndWait(ctx context.Context, preset VideoPreset, req *VideoSubmitRequest, cfg VideoFlowConfig) (*VideoFlowResult, error)
}

type DefaultVideoFlowOrchestrator struct {
	client *Client

	submitVideoTaskFn func(ctx context.Context, req VideoSubmitRequest) (*VideoSubmitResponse, error)
	videoWaitFn       func(ctx context.Context, taskID string, preset VideoPreset, opts WaitOptions) (*VideoWaitResult, error)
	downloadVideoFn   func(ctx context.Context, taskID string, videoURL string, opts FlowOptions) (string, error)
}

func NewVideoFlowOrchestrator(client *Client) VideoFlowOrchestrator {
	o := &DefaultVideoFlowOrchestrator{client: client}
	if client != nil {
		o.submitVideoTaskFn = client.SubmitVideoTask
		o.videoWaitFn = client.VideoWait
		o.downloadVideoFn = client.DownloadVideo
	}
	return o
}

func (o *DefaultVideoFlowOrchestrator) Submit(ctx context.Context, preset VideoPreset, req *VideoSubmitRequest) (*VideoSubmitResult, error) {
	if o == nil {
		return nil, fmt.Errorf("video flow orchestrator is nil")
	}
	if preset == "" {
		return nil, fmt.Errorf("preset is required")
	}
	if req == nil {
		return nil, fmt.Errorf("video submit request is required")
	}

	value := *req
	if value.Preset == "" {
		value.Preset = preset
	} else if value.Preset != preset {
		return nil, fmt.Errorf("preset mismatch: expected %q, got %q", preset, value.Preset)
	}

	if o.submitVideoTaskFn == nil {
		return nil, fmt.Errorf("client is required")
	}
	return o.submitVideoTaskFn(ctx, value)
}

func (o *DefaultVideoFlowOrchestrator) SubmitAndWait(ctx context.Context, preset VideoPreset, req *VideoSubmitRequest, cfg VideoFlowConfig) (*VideoFlowResult, error) {
	if o == nil {
		return nil, fmt.Errorf("video flow orchestrator is nil")
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	cfg.DownloadDir = strings.TrimSpace(cfg.DownloadDir)

	submitResp, err := o.Submit(ctx, preset, req)
	if err != nil {
		return nil, err
	}

	taskID := ""
	if submitResp != nil {
		taskID = strings.TrimSpace(submitResp.TaskID)
	}
	if taskID == "" {
		return nil, fmt.Errorf("submit task returned empty task_id")
	}

	result := &VideoFlowResult{TaskID: taskID}
	if !cfg.WaitEnabled {
		return result, nil
	}

	if o.videoWaitFn == nil {
		return result, fmt.Errorf("client is required")
	}

	waitResp, waitErr := o.videoWaitFn(ctx, taskID, preset, WaitOptions{Timeout: cfg.WaitTimeout})
	if waitResp != nil {
		result.Status = string(waitResp.Status)
		result.VideoURL = waitResp.VideoURL
	}
	if waitErr != nil {
		result.Error = waitErr
		if strings.TrimSpace(result.Status) == "" {
			result.Status = extractStatusFromWaitError(waitErr)
		}
		switch internalerrors.GetCode(waitErr) {
		case internalerrors.ErrBusinessFailed, internalerrors.ErrTimeout:
			return result, nil
		default:
			return result, waitErr
		}
	}

	if cfg.DownloadDir == "" {
		return result, nil
	}
	if strings.TrimSpace(result.VideoURL) == "" {
		err := fmt.Errorf("task is done but video_url is empty")
		result.Error = err
		return result, err
	}
	if o.downloadVideoFn == nil {
		return result, fmt.Errorf("client is required")
	}

	filePath, err := o.downloadVideoFn(ctx, taskID, result.VideoURL, FlowOptions{DownloadDir: cfg.DownloadDir, Overwrite: cfg.Overwrite})
	if err != nil {
		err = fmt.Errorf("task_id=%s: download video failed: %w", taskID, err)
		result.Error = err
		return result, err
	}
	result.DownloadPath = filePath
	return result, nil
}

func extractStatusFromWaitError(err error) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	const key = "status="
	idx := strings.Index(msg, key)
	if idx < 0 {
		return ""
	}
	rest := msg[idx+len(key):]
	end := len(rest)
	if j := strings.IndexAny(rest, ", )"); j >= 0 {
		end = j
	}
	return strings.TrimSpace(rest[:end])
}
