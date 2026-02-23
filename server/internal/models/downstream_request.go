package models

import (
	"fmt"
	"time"
)

type DownstreamAction string

const (
	DownstreamActionCVSync2AsyncSubmitTask DownstreamAction = "CVSync2AsyncSubmitTask"
	DownstreamActionCVSync2AsyncGetResult  DownstreamAction = "CVSync2AsyncGetResult"
)

type DownstreamRequest struct {
	ID          string           `json:"id"`
	RequestID   string           `json:"request_id"`
	APIKeyID    string           `json:"api_key_id"`
	Action      DownstreamAction `json:"action"`
	Method      string           `json:"method"`
	Path        string           `json:"path"`
	QueryString string           `json:"query_string,omitempty"`
	Headers     map[string]any   `json:"headers,omitempty"`
	Body        map[string]any   `json:"body,omitempty"`
	ClientIP    string           `json:"client_ip,omitempty"`
	ReceivedAt  time.Time        `json:"received_at"`
}

func (r DownstreamRequest) Validate() error {
	if r.ID == "" {
		return fmt.Errorf("id is required")
	}
	if r.RequestID == "" {
		return fmt.Errorf("request_id is required")
	}
	if r.APIKeyID == "" {
		return fmt.Errorf("api_key_id is required")
	}
	switch r.Action {
	case DownstreamActionCVSync2AsyncSubmitTask, DownstreamActionCVSync2AsyncGetResult:
	default:
		return fmt.Errorf("invalid action: %q", r.Action)
	}
	if r.Method == "" {
		return fmt.Errorf("method is required")
	}
	if r.Path == "" {
		return fmt.Errorf("path is required")
	}
	if r.ReceivedAt.IsZero() {
		return fmt.Errorf("received_at is required")
	}
	return nil
}
