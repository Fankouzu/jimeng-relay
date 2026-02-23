package models

import (
	"testing"
	"time"
)

func TestAPIKeyStateHelpers(t *testing.T) {
	now := time.Now().UTC()

	active := APIKey{
		ID:                  "k1",
		AccessKey:           "ak_test",
		SecretKeyHash:       "$2a$10$abcdefghijklmnopqrstuv",
		SecretKeyCiphertext: "v1:test",
		Status:              APIKeyStatusActive,
		CreatedAt:           now,
		UpdatedAt:           now,
	}
	if !active.IsActive() {
		t.Fatalf("expected active key to be active")
	}
	if active.IsExpired() {
		t.Fatalf("expected active key not expired")
	}
	if active.IsRevoked() {
		t.Fatalf("expected active key not revoked")
	}

	expiresAt := now.Add(-time.Minute)
	expired := active
	expired.ExpiresAt = &expiresAt
	if !expired.IsExpired() {
		t.Fatalf("expected key with past expires_at to be expired")
	}

	revokedAt := now.Add(-time.Second)
	revoked := active
	revoked.RevokedAt = &revokedAt
	if !revoked.IsRevoked() {
		t.Fatalf("expected key with revoked_at to be revoked")
	}
	if revoked.IsActive() {
		t.Fatalf("expected revoked key not active")
	}
}

func TestAPIKeyValidate(t *testing.T) {
	now := time.Now().UTC()

	valid := APIKey{
		ID:                  "k1",
		AccessKey:           "ak_test",
		SecretKeyHash:       "$2a$10$abcdefghijklmnopqrstuv",
		SecretKeyCiphertext: "v1:test",
		Status:              APIKeyStatusActive,
		CreatedAt:           now,
		UpdatedAt:           now,
	}

	if err := valid.Validate(); err != nil {
		t.Fatalf("expected valid api key, got error: %v", err)
	}

	invalid := valid
	invalid.SecretKeyHash = ""
	if err := invalid.Validate(); err == nil {
		t.Fatalf("expected error when secret_key_hash is empty")
	}

	invalid = valid
	invalid.SecretKeyCiphertext = ""
	if err := invalid.Validate(); err == nil {
		t.Fatalf("expected error when secret_key_ciphertext is empty")
	}

	invalid = valid
	invalid.Status = "wrong"
	if err := invalid.Validate(); err == nil {
		t.Fatalf("expected error for invalid status")
	}
}

func TestDownstreamRequestValidate(t *testing.T) {
	now := time.Now().UTC()
	request := DownstreamRequest{
		ID:          "d1",
		RequestID:   "req-1",
		APIKeyID:    "k1",
		Action:      DownstreamActionCVSync2AsyncSubmitTask,
		Method:      "POST",
		Path:        "/v1/submit",
		Headers:     map[string]any{"authorization": "***"},
		Body:        map[string]any{"prompt": "x"},
		ClientIP:    "127.0.0.1",
		ReceivedAt:  now,
		QueryString: "a=1",
	}

	if err := request.Validate(); err != nil {
		t.Fatalf("expected valid downstream request, got %v", err)
	}

	request.Method = ""
	if err := request.Validate(); err == nil {
		t.Fatalf("expected validation error for empty method")
	}
}

func TestUpstreamAttemptValidate(t *testing.T) {
	now := time.Now().UTC()
	attempt := UpstreamAttempt{
		ID:              "u1",
		RequestID:       "req-1",
		AttemptNumber:   1,
		UpstreamAction:  "CVSync2AsyncSubmitTask",
		RequestHeaders:  map[string]any{"authorization": "***"},
		RequestBody:     map[string]any{"foo": "bar"},
		ResponseStatus:  200,
		ResponseHeaders: map[string]any{"content-type": "application/json"},
		ResponseBody:    map[string]any{"ok": true},
		LatencyMs:       100,
		SentAt:          now,
	}

	if err := attempt.Validate(); err != nil {
		t.Fatalf("expected valid upstream attempt, got %v", err)
	}

	attempt.AttemptNumber = 0
	if err := attempt.Validate(); err == nil {
		t.Fatalf("expected validation error for non-positive attempt number")
	}
}

func TestAuditEventValidate(t *testing.T) {
	now := time.Now().UTC()
	event := AuditEvent{
		ID:        "a1",
		RequestID: "req-1",
		EventType: EventTypeRequestReceived,
		Actor:     "k1",
		Action:    "received request",
		Resource:  "relay.request",
		Metadata:  map[string]any{"method": "POST"},
		CreatedAt: now,
	}

	if err := event.Validate(); err != nil {
		t.Fatalf("expected valid audit event, got %v", err)
	}

	event.EventType = "bad"
	if err := event.Validate(); err == nil {
		t.Fatalf("expected validation error for invalid event type")
	}
}

func TestIdempotencyRecordValidate(t *testing.T) {
	now := time.Now().UTC()
	expiresAt := now.Add(time.Hour)
	record := IdempotencyRecord{
		ID:             "i1",
		IdempotencyKey: "idem-1",
		RequestHash:    "abc123",
		ResponseStatus: 200,
		ResponseBody:   map[string]any{"task_id": "t1"},
		CreatedAt:      now,
		ExpiresAt:      expiresAt,
	}

	if err := record.Validate(); err != nil {
		t.Fatalf("expected valid idempotency record, got %v", err)
	}

	record.RequestHash = ""
	if err := record.Validate(); err == nil {
		t.Fatalf("expected validation error when request hash is empty")
	}
}
