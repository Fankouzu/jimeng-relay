package idempotency

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	internalerrors "github.com/jimeng-relay/server/internal/errors"
	"github.com/jimeng-relay/server/internal/models"
	"github.com/jimeng-relay/server/internal/repository"
)

type fakeRepo struct {
	records          map[string]models.IdempotencyRecord
	createCalled     int
	createdRecords   []models.IdempotencyRecord
	deleteCalled     int
	deleteLastNow    time.Time
	deleteReturnN    int64
	getByKeyErr      error
	createErr        error
	deleteExpiredErr error
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{records: map[string]models.IdempotencyRecord{}}
}

func (f *fakeRepo) GetByKey(_ context.Context, idempotencyKey string) (models.IdempotencyRecord, error) {
	if f.getByKeyErr != nil {
		return models.IdempotencyRecord{}, f.getByKeyErr
	}
	rec, ok := f.records[idempotencyKey]
	if !ok {
		return models.IdempotencyRecord{}, repository.ErrNotFound
	}
	return rec, nil
}

func (f *fakeRepo) Create(_ context.Context, record models.IdempotencyRecord) error {
	f.createCalled++
	if f.createErr != nil {
		return f.createErr
	}
	f.createdRecords = append(f.createdRecords, record)
	f.records[record.IdempotencyKey] = record
	return nil
}

func (f *fakeRepo) DeleteExpired(_ context.Context, now time.Time) (int64, error) {
	f.deleteCalled++
	f.deleteLastNow = now
	if f.deleteExpiredErr != nil {
		return 0, f.deleteExpiredErr
	}
	return f.deleteReturnN, nil
}

func TestServiceResolveOrStore_ReturnsStoredResponseWhenKeyAndHashMatch(t *testing.T) {
	ctx := context.Background()
	base := time.Date(2026, 2, 24, 12, 0, 0, 0, time.UTC)
	repo := newFakeRepo()
	repo.records["idem-1"] = models.IdempotencyRecord{
		ID:             "idem_existing",
		IdempotencyKey: "idem-1",
		RequestHash:    "hash-1",
		ResponseStatus: 202,
		ResponseBody:   map[string]any{"task_id": "t-1"},
		CreatedAt:      base.Add(-time.Minute),
		ExpiresAt:      base.Add(time.Hour),
	}

	svc := NewService(repo, Config{Now: func() time.Time { return base }, TTL: 30 * time.Minute})

	got, err := svc.ResolveOrStore(ctx, ResolveRequest{
		IdempotencyKey: "idem-1",
		RequestHash:    "hash-1",
		ResponseStatus: 200,
		ResponseBody:   map[string]any{"task_id": "ignored"},
	})
	if err != nil {
		t.Fatalf("ResolveOrStore: %v", err)
	}
	if !got.Replayed {
		t.Fatalf("expected replayed response")
	}
	if got.ResponseStatus != 202 {
		t.Fatalf("expected stored status 202, got %d", got.ResponseStatus)
	}
	body, ok := got.ResponseBody.(map[string]any)
	if !ok {
		t.Fatalf("expected response body map, got %T", got.ResponseBody)
	}
	if body["task_id"] != "t-1" {
		t.Fatalf("expected stored response body")
	}
	if repo.createCalled != 0 {
		t.Fatalf("expected no create when record exists")
	}
}

func TestServiceResolveOrStore_CreatesRecordWhenKeyNotFound(t *testing.T) {
	ctx := context.Background()
	base := time.Date(2026, 2, 24, 12, 30, 0, 0, time.UTC)
	repo := newFakeRepo()
	rnd := bytes.NewReader(bytes.Repeat([]byte{0x02}, 8))

	svc := NewService(repo, Config{Now: func() time.Time { return base }, TTL: 45 * time.Minute, Random: rnd})

	got, err := svc.ResolveOrStore(ctx, ResolveRequest{
		IdempotencyKey: "idem-2",
		RequestHash:    "hash-2",
		ResponseStatus: 201,
		ResponseBody:   map[string]any{"task_id": "t-2"},
	})
	if err != nil {
		t.Fatalf("ResolveOrStore: %v", err)
	}
	if got.Replayed {
		t.Fatalf("expected non-replayed create path")
	}
	if repo.createCalled != 1 {
		t.Fatalf("expected create called once, got %d", repo.createCalled)
	}
	if len(repo.createdRecords) != 1 {
		t.Fatalf("expected 1 created record, got %d", len(repo.createdRecords))
	}
	created := repo.createdRecords[0]
	if created.IdempotencyKey != "idem-2" {
		t.Fatalf("unexpected idempotency key: %q", created.IdempotencyKey)
	}
	if created.RequestHash != "hash-2" {
		t.Fatalf("unexpected request hash: %q", created.RequestHash)
	}
	if created.ResponseStatus != 201 {
		t.Fatalf("unexpected response status: %d", created.ResponseStatus)
	}
	if !created.CreatedAt.Equal(base) {
		t.Fatalf("unexpected created_at: %s", created.CreatedAt)
	}
	if !created.ExpiresAt.Equal(base.Add(45 * time.Minute)) {
		t.Fatalf("unexpected expires_at: %s", created.ExpiresAt)
	}
	if created.ID == "" {
		t.Fatalf("expected generated id")
	}
}

func TestServiceResolveOrStore_ReturnsValidationErrorOnHashConflict(t *testing.T) {
	ctx := context.Background()
	base := time.Date(2026, 2, 24, 13, 0, 0, 0, time.UTC)
	repo := newFakeRepo()
	repo.records["idem-3"] = models.IdempotencyRecord{
		ID:             "idem_existing_3",
		IdempotencyKey: "idem-3",
		RequestHash:    "hash-old",
		ResponseStatus: 200,
		CreatedAt:      base.Add(-time.Minute),
		ExpiresAt:      base.Add(time.Hour),
	}

	svc := NewService(repo, Config{Now: func() time.Time { return base }, TTL: time.Hour})

	_, err := svc.ResolveOrStore(ctx, ResolveRequest{
		IdempotencyKey: "idem-3",
		RequestHash:    "hash-new",
		ResponseStatus: 200,
	})
	if err == nil {
		t.Fatalf("expected hash conflict error")
	}
	if internalerrors.GetCode(err) != internalerrors.ErrValidationFailed {
		t.Fatalf("expected validation error code, got %s", internalerrors.GetCode(err))
	}
	if repo.createCalled != 0 {
		t.Fatalf("expected no create on hash conflict")
	}
}

func TestServiceDeleteExpired_DelegatesToRepository(t *testing.T) {
	ctx := context.Background()
	base := time.Date(2026, 2, 24, 14, 0, 0, 0, time.UTC)
	repo := newFakeRepo()
	repo.deleteReturnN = 3

	svc := NewService(repo, Config{})

	deleted, err := svc.DeleteExpired(ctx, base)
	if err != nil {
		t.Fatalf("DeleteExpired: %v", err)
	}
	if deleted != 3 {
		t.Fatalf("expected 3 deleted rows, got %d", deleted)
	}
	if repo.deleteCalled != 1 {
		t.Fatalf("expected delete call once, got %d", repo.deleteCalled)
	}
	if !repo.deleteLastNow.Equal(base) {
		t.Fatalf("expected forwarded now, got %s", repo.deleteLastNow)
	}
}

func TestServiceDeleteExpired_ZeroNowReturnsValidationError(t *testing.T) {
	ctx := context.Background()
	repo := newFakeRepo()
	svc := NewService(repo, Config{})

	_, err := svc.DeleteExpired(ctx, time.Time{})
	if err == nil {
		t.Fatalf("expected validation error")
	}
	if internalerrors.GetCode(err) != internalerrors.ErrValidationFailed {
		t.Fatalf("expected validation error code, got %s", internalerrors.GetCode(err))
	}
	if repo.deleteCalled != 0 {
		t.Fatalf("expected repository not called for zero now")
	}
}

func TestServiceResolveOrStore_PropagatesRepositoryErrors(t *testing.T) {
	ctx := context.Background()
	base := time.Date(2026, 2, 24, 15, 0, 0, 0, time.UTC)

	t.Run("get_by_key error", func(t *testing.T) {
		repo := newFakeRepo()
		repo.getByKeyErr = errors.New("db down")
		svc := NewService(repo, Config{Now: func() time.Time { return base }, TTL: time.Hour})

		_, err := svc.ResolveOrStore(ctx, ResolveRequest{IdempotencyKey: "idem-4", RequestHash: "hash-4", ResponseStatus: 200})
		if err == nil {
			t.Fatalf("expected error")
		}
		if internalerrors.GetCode(err) != internalerrors.ErrDatabaseError {
			t.Fatalf("expected database error code, got %s", internalerrors.GetCode(err))
		}
	})

	t.Run("create error", func(t *testing.T) {
		repo := newFakeRepo()
		repo.createErr = errors.New("insert failed")
		svc := NewService(repo, Config{Now: func() time.Time { return base }, TTL: time.Hour})

		_, err := svc.ResolveOrStore(ctx, ResolveRequest{IdempotencyKey: "idem-5", RequestHash: "hash-5", ResponseStatus: 200})
		if err == nil {
			t.Fatalf("expected error")
		}
		if internalerrors.GetCode(err) != internalerrors.ErrDatabaseError {
			t.Fatalf("expected database error code, got %s", internalerrors.GetCode(err))
		}
	})
}
