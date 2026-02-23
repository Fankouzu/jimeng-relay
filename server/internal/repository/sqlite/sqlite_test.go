package sqlite

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jimeng-relay/server/internal/models"
	"github.com/jimeng-relay/server/internal/repository"
)

func newTestRepos(t *testing.T) *Repositories {
	t.Helper()
	ctx := context.Background()

	db := openTestDB(t)
	if err := ApplyMigrations(ctx, db); err != nil {
		t.Fatalf("ApplyMigrations: %v", err)
	}
	return New(db)
}

func requireNotFound(t *testing.T, err error) {
	t.Helper()
	if !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("expected repository.ErrNotFound, got: %v", err)
	}
}

func requireConstraintErr(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected constraint error, got nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, "constraint") && !strings.Contains(msg, "UNIQUE") {
		t.Fatalf("expected constraint error, got: %v", err)
	}
}

func TestAPIKeyRepo_CRUDAndQueries(t *testing.T) {
	ctx := context.Background()
	repos := newTestRepos(t)

	now := time.Date(2026, 2, 24, 3, 4, 5, 0, time.UTC)
	expiresAt := now.Add(24 * time.Hour)

	key := models.APIKey{
		ID:                  "k1",
		AccessKey:           "ak_1",
		SecretKeyHash:       "$2a$10$abcdefghijklmnopqrstuv",
		SecretKeyCiphertext: "v1:test",
		Description:         "test",
		CreatedAt:           now,
		UpdatedAt:           now,
		ExpiresAt:           &expiresAt,
		Status:              models.APIKeyStatusActive,
	}
	if err := repos.APIKeys.Create(ctx, key); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := repos.APIKeys.GetByAccessKey(ctx, "ak_1")
	if err != nil {
		t.Fatalf("GetByAccessKey: %v", err)
	}
	if got.ID != key.ID || got.AccessKey != key.AccessKey || got.SecretKeyHash != key.SecretKeyHash || got.SecretKeyCiphertext != key.SecretKeyCiphertext {
		t.Fatalf("unexpected key: %#v", got)
	}
	if got.ExpiresAt == nil || !got.ExpiresAt.Equal(expiresAt) {
		t.Fatalf("expected expires_at to roundtrip")
	}

	list, err := repos.APIKeys.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 key, got %d", len(list))
	}

	if err := repos.APIKeys.SetExpired(ctx, "k1", now.Add(2*time.Hour)); err != nil {
		t.Fatalf("SetExpired: %v", err)
	}
	got, err = repos.APIKeys.GetByAccessKey(ctx, "ak_1")
	if err != nil {
		t.Fatalf("GetByAccessKey (after set expired): %v", err)
	}
	if got.ExpiresAt == nil {
		t.Fatalf("expected expires_at to be set")
	}
	if got.Status != models.APIKeyStatusExpired {
		t.Fatalf("expected status expired, got %q", got.Status)
	}

	revokedAt := now.Add(time.Minute)
	if err := repos.APIKeys.Revoke(ctx, "k1", revokedAt); err != nil {
		t.Fatalf("Revoke: %v", err)
	}
	got, err = repos.APIKeys.GetByAccessKey(ctx, "ak_1")
	if err != nil {
		t.Fatalf("GetByAccessKey (after revoke): %v", err)
	}
	if got.RevokedAt == nil || !got.RevokedAt.Equal(revokedAt) {
		t.Fatalf("expected revoked_at to be set")
	}
	if got.Status != models.APIKeyStatusRevoked {
		t.Fatalf("expected status revoked, got %q", got.Status)
	}

	key2 := models.APIKey{
		ID:                  "k2",
		AccessKey:           "ak_2",
		SecretKeyHash:       "$2a$10$abcdefghijklmnopqrstuv",
		SecretKeyCiphertext: "v1:test2",
		CreatedAt:           now,
		UpdatedAt:           now,
		Status:              models.APIKeyStatusActive,
	}
	if err := repos.APIKeys.Create(ctx, key2); err != nil {
		t.Fatalf("Create k2: %v", err)
	}
	windowEnd := now.Add(30 * time.Minute)
	if err := repos.APIKeys.SetExpiresAt(ctx, "k2", windowEnd); err != nil {
		t.Fatalf("SetExpiresAt: %v", err)
	}
	got2, err := repos.APIKeys.GetByID(ctx, "k2")
	if err != nil {
		t.Fatalf("GetByID(k2): %v", err)
	}
	if got2.ExpiresAt == nil || !got2.ExpiresAt.Equal(windowEnd) {
		t.Fatalf("expected expires_at to be set by SetExpiresAt")
	}
	if got2.Status != models.APIKeyStatusActive {
		t.Fatalf("SetExpiresAt should keep status active, got %q", got2.Status)
	}
}

func TestAPIKeyRepo_NotFoundAndConstraints(t *testing.T) {
	ctx := context.Background()
	repos := newTestRepos(t)

	if _, err := repos.APIKeys.GetByAccessKey(ctx, "missing"); err == nil {
		t.Fatalf("expected not found error")
	} else {
		requireNotFound(t, err)
	}

	now := time.Date(2026, 2, 24, 3, 4, 5, 0, time.UTC)
	key1 := models.APIKey{ID: "k1", AccessKey: "ak_1", SecretKeyHash: "hash", SecretKeyCiphertext: "v1:test", Status: models.APIKeyStatusActive, CreatedAt: now, UpdatedAt: now}
	if err := repos.APIKeys.Create(ctx, key1); err != nil {
		t.Fatalf("Create key1: %v", err)
	}
	key2 := models.APIKey{ID: "k2", AccessKey: "ak_1", SecretKeyHash: "hash", SecretKeyCiphertext: "v1:test", Status: models.APIKeyStatusActive, CreatedAt: now, UpdatedAt: now}
	err := repos.APIKeys.Create(ctx, key2)
	requireConstraintErr(t, err)
}

func TestDownstreamRequestRepo_CRUD(t *testing.T) {
	ctx := context.Background()
	repos := newTestRepos(t)

	now := time.Date(2026, 2, 24, 3, 4, 5, 0, time.UTC)

	key := models.APIKey{ID: "k1", AccessKey: "ak_1", SecretKeyHash: "hash", SecretKeyCiphertext: "v1:test", Status: models.APIKeyStatusActive, CreatedAt: now, UpdatedAt: now}
	if err := repos.APIKeys.Create(ctx, key); err != nil {
		t.Fatalf("Create api key: %v", err)
	}

	req := models.DownstreamRequest{
		ID:          "d1",
		RequestID:   "req-1",
		APIKeyID:    "k1",
		Action:      models.DownstreamActionCVSync2AsyncSubmitTask,
		Method:      "POST",
		Path:        "/v1/submit",
		QueryString: "a=1",
		Headers:     map[string]any{"x-test": "1"},
		Body:        map[string]any{"prompt": "x"},
		ClientIP:    "127.0.0.1",
		ReceivedAt:  now,
	}
	if err := repos.DownstreamRequests.Create(ctx, req); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := repos.DownstreamRequests.GetByID(ctx, "d1")
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.RequestID != req.RequestID || got.APIKeyID != req.APIKeyID {
		t.Fatalf("unexpected request: %#v", got)
	}
	if got.Headers["x-test"] != "1" {
		t.Fatalf("expected headers to roundtrip")
	}
	if got.Body["prompt"] != "x" {
		t.Fatalf("expected body to roundtrip")
	}

	got2, err := repos.DownstreamRequests.GetByRequestID(ctx, "req-1")
	if err != nil {
		t.Fatalf("GetByRequestID: %v", err)
	}
	if got2.ID != "d1" {
		t.Fatalf("expected same row")
	}
}

func TestDownstreamRequestRepo_NotFoundAndConstraints(t *testing.T) {
	ctx := context.Background()
	repos := newTestRepos(t)

	if _, err := repos.DownstreamRequests.GetByID(ctx, "missing"); err == nil {
		t.Fatalf("expected not found error")
	} else {
		requireNotFound(t, err)
	}

	now := time.Date(2026, 2, 24, 3, 4, 5, 0, time.UTC)
	key := models.APIKey{ID: "k1", AccessKey: "ak_1", SecretKeyHash: "hash", SecretKeyCiphertext: "v1:test", Status: models.APIKeyStatusActive, CreatedAt: now, UpdatedAt: now}
	if err := repos.APIKeys.Create(ctx, key); err != nil {
		t.Fatalf("Create api key: %v", err)
	}

	req1 := models.DownstreamRequest{ID: "d1", RequestID: "req-1", APIKeyID: "k1", Action: models.DownstreamActionCVSync2AsyncSubmitTask, Method: "POST", Path: "/", ReceivedAt: now}
	if err := repos.DownstreamRequests.Create(ctx, req1); err != nil {
		t.Fatalf("Create req1: %v", err)
	}
	req2 := models.DownstreamRequest{ID: "d2", RequestID: "req-1", APIKeyID: "k1", Action: models.DownstreamActionCVSync2AsyncSubmitTask, Method: "POST", Path: "/", ReceivedAt: now}
	err := repos.DownstreamRequests.Create(ctx, req2)
	requireConstraintErr(t, err)
}

func TestUpstreamAttemptRepo_ListByRequestID(t *testing.T) {
	ctx := context.Background()
	repos := newTestRepos(t)

	now := time.Date(2026, 2, 24, 3, 4, 5, 0, time.UTC)

	if list, err := repos.UpstreamAttempts.ListByRequestID(ctx, "missing"); err != nil {
		t.Fatalf("ListByRequestID: %v", err)
	} else if len(list) != 0 {
		t.Fatalf("expected empty list")
	}

	a1 := models.UpstreamAttempt{ID: "u1", RequestID: "req-1", AttemptNumber: 1, UpstreamAction: "CVSync2AsyncSubmitTask", RequestHeaders: map[string]any{"x": "1"}, ResponseStatus: 200, ResponseBody: map[string]any{"ok": true}, LatencyMs: 10, SentAt: now}
	if err := repos.UpstreamAttempts.Create(ctx, a1); err != nil {
		t.Fatalf("Create a1: %v", err)
	}
	errStr := "boom"
	a2 := models.UpstreamAttempt{ID: "u2", RequestID: "req-1", AttemptNumber: 2, UpstreamAction: "CVSync2AsyncSubmitTask", ResponseStatus: 500, Error: &errStr, LatencyMs: 20, SentAt: now.Add(time.Second)}
	if err := repos.UpstreamAttempts.Create(ctx, a2); err != nil {
		t.Fatalf("Create a2: %v", err)
	}

	list, err := repos.UpstreamAttempts.ListByRequestID(ctx, "req-1")
	if err != nil {
		t.Fatalf("ListByRequestID: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 attempts, got %d", len(list))
	}
	if list[0].AttemptNumber != 1 || list[1].AttemptNumber != 2 {
		t.Fatalf("expected attempt ordering by attempt_number")
	}
	if list[1].Error == nil || *list[1].Error != "boom" {
		t.Fatalf("expected error to roundtrip")
	}
}

func TestAuditEventRepo_ListQueries(t *testing.T) {
	ctx := context.Background()
	repos := newTestRepos(t)

	base := time.Date(2026, 2, 24, 3, 4, 5, 0, time.UTC)

	e1 := models.AuditEvent{ID: "a1", RequestID: "req-1", EventType: models.EventTypeRequestReceived, Actor: "system", Action: "received", Resource: "relay.request", Metadata: map[string]any{"m": "1"}, CreatedAt: base}
	if err := repos.AuditEvents.Create(ctx, e1); err != nil {
		t.Fatalf("Create e1: %v", err)
	}
	e2 := models.AuditEvent{ID: "a2", RequestID: "req-1", EventType: models.EventTypeResponseSent, Actor: "system", Action: "sent", Resource: "relay.response", CreatedAt: base.Add(2 * time.Second)}
	if err := repos.AuditEvents.Create(ctx, e2); err != nil {
		t.Fatalf("Create e2: %v", err)
	}
	e3 := models.AuditEvent{ID: "a3", RequestID: "req-2", EventType: models.EventTypeError, Actor: "system", Action: "error", Resource: "relay.error", CreatedAt: base.Add(3 * time.Second)}
	if err := repos.AuditEvents.Create(ctx, e3); err != nil {
		t.Fatalf("Create e3: %v", err)
	}

	byReq, err := repos.AuditEvents.ListByRequestID(ctx, "req-1")
	if err != nil {
		t.Fatalf("ListByRequestID: %v", err)
	}
	if len(byReq) != 2 {
		t.Fatalf("expected 2 events, got %d", len(byReq))
	}
	if byReq[0].ID != "a1" || byReq[1].ID != "a2" {
		t.Fatalf("expected ordering by created_at")
	}
	if byReq[0].Metadata["m"] != "1" {
		t.Fatalf("expected metadata to roundtrip")
	}

	byTime, err := repos.AuditEvents.ListByTimeRange(ctx, base.Add(time.Second), base.Add(3*time.Second))
	if err != nil {
		t.Fatalf("ListByTimeRange: %v", err)
	}
	if len(byTime) != 2 {
		t.Fatalf("expected 2 events in range, got %d", len(byTime))
	}
	if byTime[0].ID != "a2" || byTime[1].ID != "a3" {
		t.Fatalf("unexpected events in range")
	}
}

func TestIdempotencyRecordRepo_CRUDAndDeleteExpired(t *testing.T) {
	ctx := context.Background()
	repos := newTestRepos(t)

	now := time.Date(2026, 2, 24, 3, 4, 5, 0, time.UTC)

	if _, err := repos.IdempotencyRecords.GetByKey(ctx, "missing"); err == nil {
		t.Fatalf("expected not found error")
	} else {
		requireNotFound(t, err)
	}

	r1 := models.IdempotencyRecord{ID: "i1", IdempotencyKey: "idem-1", RequestHash: "h1", ResponseStatus: 200, ResponseBody: map[string]any{"task_id": "t1"}, CreatedAt: now, ExpiresAt: now.Add(time.Minute)}
	if err := repos.IdempotencyRecords.Create(ctx, r1); err != nil {
		t.Fatalf("Create r1: %v", err)
	}
	r2 := models.IdempotencyRecord{ID: "i2", IdempotencyKey: "idem-expired", RequestHash: "h2", ResponseStatus: 200, CreatedAt: now.Add(-time.Hour), ExpiresAt: now.Add(-time.Minute)}
	if err := repos.IdempotencyRecords.Create(ctx, r2); err != nil {
		t.Fatalf("Create r2: %v", err)
	}

	got, err := repos.IdempotencyRecords.GetByKey(ctx, "idem-1")
	if err != nil {
		t.Fatalf("GetByKey: %v", err)
	}
	if got.RequestHash != "h1" || got.ResponseStatus != 200 {
		t.Fatalf("unexpected record: %#v", got)
	}
	if body, ok := got.ResponseBody.(map[string]any); !ok || body["task_id"] != "t1" {
		t.Fatalf("expected response body to roundtrip")
	}

	deleted, err := repos.IdempotencyRecords.DeleteExpired(ctx, now)
	if err != nil {
		t.Fatalf("DeleteExpired: %v", err)
	}
	if deleted != 1 {
		t.Fatalf("expected 1 deleted, got %d", deleted)
	}
	if _, err := repos.IdempotencyRecords.GetByKey(ctx, "idem-expired"); err == nil {
		t.Fatalf("expected expired record to be deleted")
	} else {
		requireNotFound(t, err)
	}
}

func TestIdempotencyRecordRepo_Constraints(t *testing.T) {
	ctx := context.Background()
	repos := newTestRepos(t)

	now := time.Date(2026, 2, 24, 3, 4, 5, 0, time.UTC)
	r1 := models.IdempotencyRecord{ID: "i1", IdempotencyKey: "idem-1", RequestHash: "h1", ResponseStatus: 200, CreatedAt: now, ExpiresAt: now.Add(time.Hour)}
	if err := repos.IdempotencyRecords.Create(ctx, r1); err != nil {
		t.Fatalf("Create r1: %v", err)
	}
	r2 := models.IdempotencyRecord{ID: "i2", IdempotencyKey: "idem-1", RequestHash: "h2", ResponseStatus: 200, CreatedAt: now, ExpiresAt: now.Add(time.Hour)}
	err := repos.IdempotencyRecords.Create(ctx, r2)
	requireConstraintErr(t, err)
}
