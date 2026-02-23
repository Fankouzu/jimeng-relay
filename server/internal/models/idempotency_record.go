package models

import (
	"fmt"
	"time"
)

type IdempotencyRecord struct {
	ID             string    `json:"id"`
	IdempotencyKey string    `json:"idempotency_key"`
	RequestHash    string    `json:"request_hash"`
	ResponseStatus int       `json:"response_status"`
	ResponseBody   any       `json:"response_body,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
	ExpiresAt      time.Time `json:"expires_at"`
}

func (r IdempotencyRecord) Validate() error {
	if r.ID == "" {
		return fmt.Errorf("id is required")
	}
	if r.IdempotencyKey == "" {
		return fmt.Errorf("idempotency_key is required")
	}
	if r.RequestHash == "" {
		return fmt.Errorf("request_hash is required")
	}
	if r.ResponseStatus < 0 {
		return fmt.Errorf("response_status must be zero or positive")
	}
	if r.CreatedAt.IsZero() {
		return fmt.Errorf("created_at is required")
	}
	if r.ExpiresAt.IsZero() {
		return fmt.Errorf("expires_at is required")
	}
	if !r.ExpiresAt.After(r.CreatedAt) {
		return fmt.Errorf("expires_at must be after created_at")
	}
	return nil
}
