//go:build integration

package postgres

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	internalerrors "github.com/jimeng-relay/server/internal/errors"
	"github.com/jimeng-relay/server/internal/models"
	"github.com/jimeng-relay/server/internal/repository"
)

func openIntegrationDB(t *testing.T) *DB {
	t.Helper()
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("DATABASE_URL not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	t.Cleanup(cancel)

	db, err := Open(ctx, dbURL)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(db.Close)

	cleanup := func() {
		cctx, ccancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer ccancel()
		_, _ = db.pool.Exec(cctx, `TRUNCATE TABLE audit_events, upstream_attempts, downstream_requests, idempotency_records, api_keys CASCADE`)
	}
	cleanup()
	t.Cleanup(cleanup)

	return db
}

func TestOpen_InvalidURL(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	if _, err := Open(ctx, "not-a-url"); err == nil {
		t.Fatalf("expected error")
	}
}

func TestOpen_PingFailure(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	_, err := Open(ctx, "postgres://invalid:invalid@127.0.0.1:1/doesnotmatter")
	if err == nil {
		t.Fatalf("expected error")
	}
}

func TestMigrate_Idempotent(t *testing.T) {
	db := openIntegrationDB(t)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	t.Cleanup(cancel)

	if err := Migrate(ctx, db.pool); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	if err := Migrate(ctx, db.pool); err != nil {
		t.Fatalf("Migrate second time: %v", err)
	}
}

func TestAPIKeyRepository_CRUD(t *testing.T) {
	db := openIntegrationDB(t)
	repo := db.APIKeys()

	now := time.Now().UTC().Truncate(time.Microsecond)
	key := models.APIKey{
		ID:                  "k1",
		AccessKey:           "ak_test",
		SecretKeyHash:       "$2a$10$abcdefghijklmnopqrstuv",
		SecretKeyCiphertext: "v1:test",
		Description:         "test",
		Status:              models.APIKeyStatusActive,
		CreatedAt:           now,
		UpdatedAt:           now,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	t.Cleanup(cancel)

	if err := repo.Create(ctx, key); err != nil {
		t.Fatalf("Create: %v", err)
	}

	fetched, err := repo.GetByAccessKey(ctx, key.AccessKey)
	if err != nil {
		t.Fatalf("GetByAccessKey: %v", err)
	}
	if fetched.ID != key.ID {
		t.Fatalf("expected id %q, got %q", key.ID, fetched.ID)
	}
	if fetched.Status != models.APIKeyStatusActive {
		t.Fatalf("expected status active, got %q", fetched.Status)
	}

	keys, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(keys) != 1 {
		t.Fatalf("expected 1 key, got %d", len(keys))
	}

	revokedAt := now.Add(time.Minute)
	if err := repo.Revoke(ctx, key.ID, revokedAt); err != nil {
		t.Fatalf("Revoke: %v", err)
	}

	key2 := models.APIKey{
		ID:                  "k2",
		AccessKey:           "ak_test2",
		SecretKeyHash:       "$2a$10$abcdefghijklmnopqrstuv",
		SecretKeyCiphertext: "v1:test2",
		Status:              models.APIKeyStatusActive,
		CreatedAt:           now,
		UpdatedAt:           now,
	}
	if err := repo.Create(ctx, key2); err != nil {
		t.Fatalf("Create 2: %v", err)
	}

	expiredAt := now.Add(time.Hour)
	if err := repo.SetExpired(ctx, key2.ID, expiredAt); err != nil {
		t.Fatalf("SetExpired: %v", err)
	}

	key3 := models.APIKey{
		ID:                  "k3",
		AccessKey:           "ak_test3",
		SecretKeyHash:       "$2a$10$abcdefghijklmnopqrstuv",
		SecretKeyCiphertext: "v1:test3",
		Status:              models.APIKeyStatusActive,
		CreatedAt:           now,
		UpdatedAt:           now,
	}
	if err := repo.Create(ctx, key3); err != nil {
		t.Fatalf("Create 3: %v", err)
	}
	windowEnd := now.Add(10 * time.Minute)
	if err := repo.SetExpiresAt(ctx, key3.ID, windowEnd); err != nil {
		t.Fatalf("SetExpiresAt: %v", err)
	}
	fetched3, err := repo.GetByID(ctx, key3.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if fetched3.ExpiresAt == nil || !fetched3.ExpiresAt.Equal(windowEnd) {
		t.Fatalf("expected expires_at to be set")
	}
	if fetched3.Status != models.APIKeyStatusActive {
		t.Fatalf("SetExpiresAt should not force expired status")
	}
}

func TestAPIKeyRepository_NotFound(t *testing.T) {
	db := openIntegrationDB(t)
	repo := db.APIKeys()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	t.Cleanup(cancel)

	if _, err := repo.GetByAccessKey(ctx, "missing"); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
	if err := repo.Revoke(ctx, "missing-id", time.Now().UTC()); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
	if err := repo.SetExpired(ctx, "missing-id", time.Now().UTC()); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestAPIKeyRepository_ConstraintViolation(t *testing.T) {
	db := openIntegrationDB(t)
	repo := db.APIKeys()

	now := time.Now().UTC()
	key := models.APIKey{
		ID:                  "k1",
		AccessKey:           "ak_dup",
		SecretKeyHash:       "$2a$10$abcdefghijklmnopqrstuv",
		SecretKeyCiphertext: "v1:test",
		Status:              models.APIKeyStatusActive,
		CreatedAt:           now,
		UpdatedAt:           now,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	t.Cleanup(cancel)

	if err := repo.Create(ctx, key); err != nil {
		t.Fatalf("Create: %v", err)
	}
	key2 := key
	key2.ID = "k2"
	if err := repo.Create(ctx, key2); err == nil {
		t.Fatalf("expected error")
	}
}

func TestDownstreamRequestRepository_CRUD(t *testing.T) {
	db := openIntegrationDB(t)
	keys := db.APIKeys()
	reqRepo := db.DownstreamRequests()

	now := time.Now().UTC()
	key := models.APIKey{
		ID:                  "k1",
		AccessKey:           "ak_test",
		SecretKeyHash:       "$2a$10$abcdefghijklmnopqrstuv",
		SecretKeyCiphertext: "v1:test",
		Status:              models.APIKeyStatusActive,
		CreatedAt:           now,
		UpdatedAt:           now,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	t.Cleanup(cancel)

	if err := keys.Create(ctx, key); err != nil {
		t.Fatalf("Create key: %v", err)
	}

	req := models.DownstreamRequest{
		ID:          "d1",
		RequestID:   "req-1",
		APIKeyID:    key.ID,
		Action:      models.DownstreamActionCVSync2AsyncSubmitTask,
		Method:      "POST",
		Path:        "/v1/submit",
		QueryString: "a=1",
		Headers:     map[string]any{"x": "y"},
		Body:        map[string]any{"prompt": "hello"},
		ClientIP:    "127.0.0.1",
		ReceivedAt:  now,
	}
	if err := reqRepo.Create(ctx, req); err != nil {
		t.Fatalf("Create: %v", err)
	}

	byID, err := reqRepo.GetByID(ctx, req.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if byID.RequestID != req.RequestID {
		t.Fatalf("expected request_id %q, got %q", req.RequestID, byID.RequestID)
	}
	if byID.Headers["x"] != "y" {
		t.Fatalf("expected header")
	}

	byReqID, err := reqRepo.GetByRequestID(ctx, req.RequestID)
	if err != nil {
		t.Fatalf("GetByRequestID: %v", err)
	}
	if byReqID.ID != req.ID {
		t.Fatalf("expected id %q, got %q", req.ID, byReqID.ID)
	}
}

func TestDownstreamRequestRepository_NotFound(t *testing.T) {
	db := openIntegrationDB(t)
	reqRepo := db.DownstreamRequests()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	t.Cleanup(cancel)

	if _, err := reqRepo.GetByID(ctx, "missing"); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
	if _, err := reqRepo.GetByRequestID(ctx, "missing"); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestUpstreamAttemptRepository_CreateAndList(t *testing.T) {
	db := openIntegrationDB(t)
	keys := db.APIKeys()
	reqRepo := db.DownstreamRequests()
	attemptRepo := db.UpstreamAttempts()

	now := time.Now().UTC()
	key := models.APIKey{
		ID:                  "k1",
		AccessKey:           "ak_test",
		SecretKeyHash:       "$2a$10$abcdefghijklmnopqrstuv",
		SecretKeyCiphertext: "v1:test",
		Status:              models.APIKeyStatusActive,
		CreatedAt:           now,
		UpdatedAt:           now,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	t.Cleanup(cancel)

	if err := keys.Create(ctx, key); err != nil {
		t.Fatalf("Create key: %v", err)
	}
	if err := reqRepo.Create(ctx, models.DownstreamRequest{
		ID:         "d1",
		RequestID:  "req-1",
		APIKeyID:   key.ID,
		Action:     models.DownstreamActionCVSync2AsyncSubmitTask,
		Method:     "POST",
		Path:       "/",
		ReceivedAt: now,
	}); err != nil {
		t.Fatalf("Create downstream request: %v", err)
	}

	a1 := models.UpstreamAttempt{
		ID:              "u1",
		RequestID:       "req-1",
		AttemptNumber:   1,
		UpstreamAction:  "CVSync2AsyncSubmitTask",
		RequestHeaders:  map[string]any{"a": "b"},
		RequestBody:     map[string]any{"x": 1},
		ResponseStatus:  200,
		ResponseHeaders: map[string]any{"content-type": "application/json"},
		ResponseBody:    map[string]any{"ok": true},
		LatencyMs:       12,
		SentAt:          now,
	}
	if err := attemptRepo.Create(ctx, a1); err != nil {
		t.Fatalf("Create attempt: %v", err)
	}

	list, err := attemptRepo.ListByRequestID(ctx, "req-1")
	if err != nil {
		t.Fatalf("ListByRequestID: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 attempt, got %d", len(list))
	}
	if list[0].AttemptNumber != 1 {
		t.Fatalf("expected attempt_number 1")
	}
}

func TestAuditEventRepository_CreateAndList(t *testing.T) {
	db := openIntegrationDB(t)
	repo := db.AuditEvents()

	now := time.Now().UTC()
	e1 := models.AuditEvent{
		ID:        "a1",
		RequestID: "req-1",
		EventType: models.EventTypeRequestReceived,
		Actor:     "system",
		Action:    "received",
		Resource:  "relay.request",
		Metadata:  map[string]any{"m": "n"},
		CreatedAt: now.Add(-time.Minute),
	}
	e2 := e1
	e2.ID = "a2"
	e2.EventType = models.EventTypeResponseSent
	e2.CreatedAt = now

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	t.Cleanup(cancel)

	if err := repo.Create(ctx, e1); err != nil {
		t.Fatalf("Create 1: %v", err)
	}
	if err := repo.Create(ctx, e2); err != nil {
		t.Fatalf("Create 2: %v", err)
	}

	byReq, err := repo.ListByRequestID(ctx, "req-1")
	if err != nil {
		t.Fatalf("ListByRequestID: %v", err)
	}
	if len(byReq) != 2 {
		t.Fatalf("expected 2 events, got %d", len(byReq))
	}

	byRange, err := repo.ListByTimeRange(ctx, now.Add(-2*time.Minute), now)
	if err != nil {
		t.Fatalf("ListByTimeRange: %v", err)
	}
	if len(byRange) != 2 {
		t.Fatalf("expected 2 events, got %d", len(byRange))
	}
}

func TestIdempotencyRecordRepository_CRUDAndDeleteExpired(t *testing.T) {
	db := openIntegrationDB(t)
	repo := db.IdempotencyRecords()

	now := time.Now().UTC()
	rec := models.IdempotencyRecord{
		ID:             "i1",
		IdempotencyKey: "idem-1",
		RequestHash:    "hash",
		ResponseStatus: 200,
		ResponseBody:   map[string]any{"task_id": "t1"},
		CreatedAt:      now,
		ExpiresAt:      now.Add(time.Hour),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	t.Cleanup(cancel)

	if err := repo.Create(ctx, rec); err != nil {
		t.Fatalf("Create: %v", err)
	}

	fetched, err := repo.GetByKey(ctx, rec.IdempotencyKey)
	if err != nil {
		t.Fatalf("GetByKey: %v", err)
	}
	if fetched.ID != rec.ID {
		t.Fatalf("expected id %q, got %q", rec.ID, fetched.ID)
	}

	deleted, err := repo.DeleteExpired(ctx, now)
	if err != nil {
		t.Fatalf("DeleteExpired: %v", err)
	}
	if deleted != 0 {
		t.Fatalf("expected 0 deleted, got %d", deleted)
	}

	deleted, err = repo.DeleteExpired(ctx, now.Add(2*time.Hour))
	if err != nil {
		t.Fatalf("DeleteExpired 2: %v", err)
	}
	if deleted != 1 {
		t.Fatalf("expected 1 deleted, got %d", deleted)
	}
}

func TestRepositoryErrors_IsNotFound(t *testing.T) {
	if repository.IsNotFound(nil) {
		t.Fatalf("expected false")
	}
	if !repository.IsNotFound(repository.ErrNotFound) {
		t.Fatalf("expected true")
	}
	if repository.IsNotFound(internalerrors.New(internalerrors.ErrDatabaseError, "x", nil)) {
		t.Fatalf("expected false")
	}
}
