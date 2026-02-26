package jimeng

import (
	"context"
	"fmt"
	"time"

	"github.com/jimeng-relay/client/internal/api"
	internalerrors "github.com/jimeng-relay/client/internal/errors"
)

type WaitOptions struct {
	Interval time.Duration
	Timeout  time.Duration
}

type WaitResult struct {
	FinalStatus      TaskStatus
	ImageURLs        []string
	BinaryDataBase64 []string
	PollCount        int
}

func (c *Client) Wait(ctx context.Context, taskID string, opts WaitOptions) (*WaitResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, internalerrors.New(internalerrors.ErrTimeout, "context done before wait", err)
	}
	if taskID == "" {
		return nil, internalerrors.New(internalerrors.ErrValidationFailed, "taskID is required", nil)
	}

	interval := opts.Interval
	if interval <= 0 {
		interval = 2 * time.Second
	}
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = 60 * time.Second
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	timer := time.NewTimer(timeout)
	defer func() {
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
	}()

	pollCount := 0
	var last *GetResultResponse

	for {
		resp, err := c.GetResult(ctx, GetResultRequest{TaskID: taskID})
		if err != nil {
			return nil, err
		}
		pollCount++
		last = resp

		if resp.IsTerminal() {
			return &WaitResult{
				FinalStatus:      resp.Status,
				ImageURLs:        resp.ImageURLs,
				BinaryDataBase64: resp.BinaryDataBase64,
				PollCount:        pollCount,
			}, nil
		}

		select {
		case <-ctx.Done():
			return nil, internalerrors.New(internalerrors.ErrTimeout, "context done during wait", ctx.Err())
		case <-timer.C:
			status := TaskStatus("")
			if last != nil {
				status = last.Status
			}
			return nil, internalerrors.New(
				internalerrors.ErrTimeout,
				fmt.Sprintf("wait timeout after %s (status=%s, polls=%d)", timeout, status, pollCount),
				context.DeadlineExceeded,
			)
		case <-ticker.C:
		}
	}
}

type VideoWaitResult struct {
	Status    VideoStatus
	VideoURL  string
	PollCount int
}

func (c *Client) VideoWait(ctx context.Context, taskID string, preset api.VideoPreset, opts WaitOptions) (*VideoWaitResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, internalerrors.New(internalerrors.ErrTimeout, "context done before wait", err)
	}
	if taskID == "" {
		return nil, internalerrors.New(internalerrors.ErrValidationFailed, "taskID is required", nil)
	}
	if preset == "" {
		return nil, internalerrors.New(internalerrors.ErrValidationFailed, "preset is required", nil)
	}

	reqKey, err := api.VideoQueryReqKey(preset)
	if err != nil {
		return nil, internalerrors.New(internalerrors.ErrValidationFailed, "invalid preset", err)
	}

	interval := opts.Interval
	if interval <= 0 {
		interval = 2 * time.Second
	}
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = 5 * time.Minute
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	timer := time.NewTimer(timeout)
	defer func() {
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
	}()

	pollCount := 0
	var last *VideoGetResultResponse

	for {
		resp, err := c.getVideoResult(ctx, VideoGetResultRequest{TaskID: taskID, Preset: preset, ReqKey: reqKey})
		if err != nil {
			return nil, err
		}
		pollCount++
		last = resp

		if resp.Status == VideoStatusDone {
			return &VideoWaitResult{Status: resp.Status, VideoURL: resp.VideoURL, PollCount: pollCount}, nil
		}
		if resp.Status == VideoStatusFailed || resp.Status == VideoStatusNotFound || resp.Status == VideoStatusExpired {
			return nil, internalerrors.New(
				internalerrors.ErrBusinessFailed,
				fmt.Sprintf("video task ended with status=%s (polls=%d)", resp.Status, pollCount),
				nil,
			)
		}

		select {
		case <-ctx.Done():
			return nil, internalerrors.New(internalerrors.ErrTimeout, "context done during wait", ctx.Err())
		case <-timer.C:
			status := VideoStatus("")
			if last != nil {
				status = last.Status
			}
			return nil, internalerrors.New(
				internalerrors.ErrTimeout,
				fmt.Sprintf("wait timeout after %s (status=%s, polls=%d)", timeout, status, pollCount),
				context.DeadlineExceeded,
			)
		case <-ticker.C:
		}
	}
}

func (c *Client) getVideoResult(ctx context.Context, req VideoGetResultRequest) (*VideoGetResultResponse, error) {
	if c.videoGetResult != nil {
		return c.videoGetResult(ctx, req)
	}
	return c.GetVideoResult(ctx, req)
}
