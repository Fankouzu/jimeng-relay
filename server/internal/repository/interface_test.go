package repository

import (
	"context"
	"testing"
	"time"

	"github.com/jimeng-relay/server/internal/models"
)

type mockAPIKeyRepository struct{}

func (m *mockAPIKeyRepository) Create(_ context.Context, key models.APIKey) error {
	return key.Validate()
}

func (m *mockAPIKeyRepository) GetByAccessKey(_ context.Context, accessKey string) (models.APIKey, error) {
	return models.APIKey{ID: "k1", AccessKey: accessKey, SecretKeyHash: "$2a$10$abcdefghijklmnopqrstuv", Status: models.APIKeyStatusActive, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}, nil
}

func (m *mockAPIKeyRepository) List(_ context.Context) ([]models.APIKey, error) {
	return []models.APIKey{}, nil
}

func (m *mockAPIKeyRepository) Revoke(_ context.Context, _ string, revokedAt time.Time) error {
	if revokedAt.IsZero() {
		return context.DeadlineExceeded
	}
	return nil
}

func (m *mockAPIKeyRepository) SetExpired(_ context.Context, _ string, expiredAt time.Time) error {
	if expiredAt.IsZero() {
		return context.DeadlineExceeded
	}
	return nil
}

type mockDownstreamRequestRepository struct{}

func (m *mockDownstreamRequestRepository) Create(_ context.Context, request models.DownstreamRequest) error {
	return request.Validate()
}

func (m *mockDownstreamRequestRepository) GetByID(_ context.Context, id string) (models.DownstreamRequest, error) {
	return models.DownstreamRequest{ID: id, RequestID: "req-1", APIKeyID: "k1", Action: models.DownstreamActionCVSync2AsyncSubmitTask, Method: "POST", Path: "/", ReceivedAt: time.Now().UTC()}, nil
}

func (m *mockDownstreamRequestRepository) GetByRequestID(_ context.Context, requestID string) (models.DownstreamRequest, error) {
	return models.DownstreamRequest{ID: "d1", RequestID: requestID, APIKeyID: "k1", Action: models.DownstreamActionCVSync2AsyncSubmitTask, Method: "POST", Path: "/", ReceivedAt: time.Now().UTC()}, nil
}

type mockUpstreamAttemptRepository struct{}

func (m *mockUpstreamAttemptRepository) Create(_ context.Context, attempt models.UpstreamAttempt) error {
	return attempt.Validate()
}

func (m *mockUpstreamAttemptRepository) ListByRequestID(_ context.Context, _ string) ([]models.UpstreamAttempt, error) {
	return []models.UpstreamAttempt{}, nil
}

type mockAuditEventRepository struct{}

func (m *mockAuditEventRepository) Create(_ context.Context, event models.AuditEvent) error {
	return event.Validate()
}

func (m *mockAuditEventRepository) ListByRequestID(_ context.Context, _ string) ([]models.AuditEvent, error) {
	return []models.AuditEvent{}, nil
}

func (m *mockAuditEventRepository) ListByTimeRange(_ context.Context, _, _ time.Time) ([]models.AuditEvent, error) {
	return []models.AuditEvent{}, nil
}

type mockIdempotencyRecordRepository struct{}

func (m *mockIdempotencyRecordRepository) GetByKey(_ context.Context, key string) (models.IdempotencyRecord, error) {
	return models.IdempotencyRecord{ID: "i1", IdempotencyKey: key, RequestHash: "hash", ResponseStatus: 200, CreatedAt: time.Now().UTC(), ExpiresAt: time.Now().UTC().Add(time.Hour)}, nil
}

func (m *mockIdempotencyRecordRepository) Create(_ context.Context, record models.IdempotencyRecord) error {
	return record.Validate()
}

func (m *mockIdempotencyRecordRepository) DeleteExpired(_ context.Context, _ time.Time) (int64, error) {
	return 0, nil
}

func TestRepositoryContracts(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()

	var apiKeyRepo APIKeyRepository = &mockAPIKeyRepository{}
	if err := apiKeyRepo.Create(ctx, models.APIKey{ID: "k1", AccessKey: "ak", SecretKeyHash: "$2a$10$abcdefghijklmnopqrstuv", Status: models.APIKeyStatusActive, CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("unexpected error creating api key: %v", err)
	}
	if _, err := apiKeyRepo.GetByAccessKey(ctx, "ak"); err != nil {
		t.Fatalf("unexpected error getting api key: %v", err)
	}
	if _, err := apiKeyRepo.List(ctx); err != nil {
		t.Fatalf("unexpected error listing api keys: %v", err)
	}

	var downstreamRepo DownstreamRequestRepository = &mockDownstreamRequestRepository{}
	if err := downstreamRepo.Create(ctx, models.DownstreamRequest{ID: "d1", RequestID: "req-1", APIKeyID: "k1", Action: models.DownstreamActionCVSync2AsyncSubmitTask, Method: "POST", Path: "/", ReceivedAt: now}); err != nil {
		t.Fatalf("unexpected error creating downstream request: %v", err)
	}
	if _, err := downstreamRepo.GetByID(ctx, "d1"); err != nil {
		t.Fatalf("unexpected error getting downstream request by id: %v", err)
	}
	if _, err := downstreamRepo.GetByRequestID(ctx, "req-1"); err != nil {
		t.Fatalf("unexpected error getting downstream request by request id: %v", err)
	}

	var upstreamRepo UpstreamAttemptRepository = &mockUpstreamAttemptRepository{}
	if err := upstreamRepo.Create(ctx, models.UpstreamAttempt{ID: "u1", RequestID: "req-1", AttemptNumber: 1, UpstreamAction: "CVSync2AsyncSubmitTask", ResponseStatus: 200, SentAt: now}); err != nil {
		t.Fatalf("unexpected error creating upstream attempt: %v", err)
	}
	if _, err := upstreamRepo.ListByRequestID(ctx, "req-1"); err != nil {
		t.Fatalf("unexpected error listing upstream attempts: %v", err)
	}

	var auditRepo AuditEventRepository = &mockAuditEventRepository{}
	if err := auditRepo.Create(ctx, models.AuditEvent{ID: "a1", RequestID: "req-1", EventType: models.EventTypeRequestReceived, Actor: "system", Action: "received", Resource: "relay.request", CreatedAt: now}); err != nil {
		t.Fatalf("unexpected error creating audit event: %v", err)
	}
	if _, err := auditRepo.ListByRequestID(ctx, "req-1"); err != nil {
		t.Fatalf("unexpected error listing audit events by request id: %v", err)
	}
	if _, err := auditRepo.ListByTimeRange(ctx, now.Add(-time.Hour), now); err != nil {
		t.Fatalf("unexpected error listing audit events by time range: %v", err)
	}

	var idempotencyRepo IdempotencyRecordRepository = &mockIdempotencyRecordRepository{}
	if _, err := idempotencyRepo.GetByKey(ctx, "idem-1"); err != nil {
		t.Fatalf("unexpected error getting idempotency record: %v", err)
	}
	if err := idempotencyRepo.Create(ctx, models.IdempotencyRecord{ID: "i1", IdempotencyKey: "idem-1", RequestHash: "hash", ResponseStatus: 200, CreatedAt: now, ExpiresAt: now.Add(time.Hour)}); err != nil {
		t.Fatalf("unexpected error creating idempotency record: %v", err)
	}
	if _, err := idempotencyRepo.DeleteExpired(ctx, now); err != nil {
		t.Fatalf("unexpected error deleting expired idempotency records: %v", err)
	}
}
