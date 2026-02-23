package models

import (
	"fmt"
	"time"
)

type UpstreamAttempt struct {
	ID              string         `json:"id"`
	RequestID       string         `json:"request_id"`
	AttemptNumber   int            `json:"attempt_number"`
	UpstreamAction  string         `json:"upstream_action"`
	RequestHeaders  map[string]any `json:"request_headers,omitempty"`
	RequestBody     map[string]any `json:"request_body,omitempty"`
	ResponseStatus  int            `json:"response_status"`
	ResponseHeaders map[string]any `json:"response_headers,omitempty"`
	ResponseBody    any            `json:"response_body,omitempty"`
	LatencyMs       int64          `json:"latency_ms"`
	Error           *string        `json:"error,omitempty"`
	SentAt          time.Time      `json:"sent_at"`
}

func (a UpstreamAttempt) Validate() error {
	if a.ID == "" {
		return fmt.Errorf("id is required")
	}
	if a.RequestID == "" {
		return fmt.Errorf("request_id is required")
	}
	if a.AttemptNumber <= 0 {
		return fmt.Errorf("attempt_number must be greater than zero")
	}
	if a.UpstreamAction == "" {
		return fmt.Errorf("upstream_action is required")
	}
	if a.ResponseStatus < 0 {
		return fmt.Errorf("response_status must be zero or positive")
	}
	if a.LatencyMs < 0 {
		return fmt.Errorf("latency_ms must be zero or positive")
	}
	if a.SentAt.IsZero() {
		return fmt.Errorf("sent_at is required")
	}
	return nil
}
