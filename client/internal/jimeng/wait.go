package jimeng

import (
	"context"
	"fmt"
	"time"

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
