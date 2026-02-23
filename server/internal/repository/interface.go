package repository

import (
	"context"
	"time"

	"github.com/jimeng-relay/server/internal/models"
)

type APIKeyRepository interface {
	Create(ctx context.Context, key models.APIKey) error
	GetByAccessKey(ctx context.Context, accessKey string) (models.APIKey, error)
	List(ctx context.Context) ([]models.APIKey, error)
	Revoke(ctx context.Context, id string, revokedAt time.Time) error
	SetExpired(ctx context.Context, id string, expiredAt time.Time) error
}

type DownstreamRequestRepository interface {
	Create(ctx context.Context, request models.DownstreamRequest) error
	GetByID(ctx context.Context, id string) (models.DownstreamRequest, error)
	GetByRequestID(ctx context.Context, requestID string) (models.DownstreamRequest, error)
}

type UpstreamAttemptRepository interface {
	Create(ctx context.Context, attempt models.UpstreamAttempt) error
	ListByRequestID(ctx context.Context, requestID string) ([]models.UpstreamAttempt, error)
}

type AuditEventRepository interface {
	Create(ctx context.Context, event models.AuditEvent) error
	ListByRequestID(ctx context.Context, requestID string) ([]models.AuditEvent, error)
	ListByTimeRange(ctx context.Context, start, end time.Time) ([]models.AuditEvent, error)
}

type IdempotencyRecordRepository interface {
	GetByKey(ctx context.Context, idempotencyKey string) (models.IdempotencyRecord, error)
	Create(ctx context.Context, record models.IdempotencyRecord) error
	DeleteExpired(ctx context.Context, now time.Time) (int64, error)
}
