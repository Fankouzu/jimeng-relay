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

	resp, pollCount, err := poll(ctx, interval, timeout,
		func() (*GetResultResponse, error) {
			return c.GetResult(ctx, GetResultRequest{TaskID: taskID})
		},
		func(resp *GetResultResponse, _ int) (bool, error) {
			return resp.IsTerminal(), nil
		},
		func(resp *GetResultResponse) string {
			if resp == nil {
				return ""
			}
			return string(resp.Status)
		},
	)
	if err != nil {
		return nil, err
	}

	return &WaitResult{
		FinalStatus:      resp.Status,
		ImageURLs:        resp.ImageURLs,
		BinaryDataBase64: resp.BinaryDataBase64,
		PollCount:        pollCount,
	}, nil
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

	resp, pollCount, err := poll(ctx, interval, timeout,
		func() (*VideoGetResultResponse, error) {
			return c.getVideoResult(ctx, VideoGetResultRequest{TaskID: taskID, Preset: preset, ReqKey: reqKey})
		},
		func(resp *VideoGetResultResponse, pc int) (bool, error) {
			if resp.Status == VideoStatusDone {
				return true, nil
			}
			if resp.Status == VideoStatusFailed || resp.Status == VideoStatusNotFound || resp.Status == VideoStatusExpired {
				return false, internalerrors.New(
					internalerrors.ErrBusinessFailed,
					fmt.Sprintf("video task ended with status=%s (polls=%d)", resp.Status, pc),
					nil,
				)
			}
			return false, nil
		},
		func(resp *VideoGetResultResponse) string {
			if resp == nil {
				return ""
			}
			return string(resp.Status)
		},
	)
	if err != nil {
		return nil, err
	}

	return &VideoWaitResult{Status: resp.Status, VideoURL: resp.VideoURL, PollCount: pollCount}, nil
}


func (c *Client) getVideoResult(ctx context.Context, req VideoGetResultRequest) (*VideoGetResultResponse, error) {
	if c.videoGetResult != nil {
		return c.videoGetResult(ctx, req)
	}
	return c.GetVideoResult(ctx, req)
}
func poll[T any](
	ctx context.Context,
	interval, timeout time.Duration,
	fetch func() (T, error),
	checkTerminal func(T, int) (bool, error),
	getStatus func(T) string,
) (T, int, error) {
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
	var last T
	var zero T

	for {
		resp, err := fetch()
		if err != nil {
			return zero, 0, err
		}
		pollCount++
		last = resp

		terminal, err := checkTerminal(resp, pollCount)
		if err != nil {
			return zero, 0, err
		}
		if terminal {
			return resp, pollCount, nil
		}

		select {
		case <-ctx.Done():
			return zero, 0, internalerrors.New(internalerrors.ErrTimeout, "context done during wait", ctx.Err())
		case <-timer.C:
			status := getStatus(last)
			return zero, 0, internalerrors.New(
				internalerrors.ErrTimeout,
				fmt.Sprintf("wait timeout after %s (status=%s, polls=%d)", timeout, status, pollCount),
				context.DeadlineExceeded,
			)
		case <-ticker.C:
		}
	}
}
