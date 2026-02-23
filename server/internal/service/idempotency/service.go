package idempotency

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"io"
	"strings"
	"time"

	internalerrors "github.com/jimeng-relay/server/internal/errors"
	"github.com/jimeng-relay/server/internal/models"
	"github.com/jimeng-relay/server/internal/repository"
)

const defaultTTL = 24 * time.Hour

type Config struct {
	Now    func() time.Time
	TTL    time.Duration
	Random io.Reader
}

type Service struct {
	repo   repository.IdempotencyRecordRepository
	now    func() time.Time
	ttl    time.Duration
	random io.Reader
}

type ResolveRequest struct {
	IdempotencyKey string
	RequestHash    string
	ResponseStatus int
	ResponseBody   any
}

type ResolveResult struct {
	Replayed       bool
	ResponseStatus int
	ResponseBody   any
}

func NewService(repo repository.IdempotencyRecordRepository, cfg Config) *Service {
	nowFn := cfg.Now
	if nowFn == nil {
		nowFn = func() time.Time { return time.Now().UTC() }
	}
	ttl := cfg.TTL
	if ttl <= 0 {
		ttl = defaultTTL
	}
	rnd := cfg.Random
	if rnd == nil {
		rnd = rand.Reader
	}
	return &Service{repo: repo, now: nowFn, ttl: ttl, random: rnd}
}

func (s *Service) ResolveOrStore(ctx context.Context, req ResolveRequest) (ResolveResult, error) {
	if s.repo == nil {
		return ResolveResult{}, internalerrors.New(internalerrors.ErrInternalError, "idempotency repository is required", nil)
	}

	req.IdempotencyKey = strings.TrimSpace(req.IdempotencyKey)
	req.RequestHash = strings.TrimSpace(req.RequestHash)
	if req.IdempotencyKey == "" {
		return ResolveResult{}, internalerrors.New(internalerrors.ErrValidationFailed, "idempotency_key is required", nil)
	}
	if req.RequestHash == "" {
		return ResolveResult{}, internalerrors.New(internalerrors.ErrValidationFailed, "request_hash is required", nil)
	}
	if req.ResponseStatus < 0 {
		return ResolveResult{}, internalerrors.New(internalerrors.ErrValidationFailed, "response_status must be zero or positive", nil)
	}

	now := s.now().UTC()
	rec, err := s.repo.GetByKey(ctx, req.IdempotencyKey)
	if err == nil {
		if !rec.ExpiresAt.After(now) {
			return ResolveResult{}, internalerrors.New(internalerrors.ErrValidationFailed, "idempotency key has expired", nil)
		}
		if rec.RequestHash != req.RequestHash {
			return ResolveResult{}, internalerrors.New(internalerrors.ErrValidationFailed, "idempotency key request hash mismatch", nil)
		}
		return ResolveResult{Replayed: true, ResponseStatus: rec.ResponseStatus, ResponseBody: rec.ResponseBody}, nil
	}
	if !repository.IsNotFound(err) {
		return ResolveResult{}, internalerrors.New(internalerrors.ErrDatabaseError, "get idempotency record", err)
	}

	id, err := generateID(s.random)
	if err != nil {
		return ResolveResult{}, internalerrors.New(internalerrors.ErrInternalError, "generate idempotency record id", err)
	}
	rec = models.IdempotencyRecord{
		ID:             id,
		IdempotencyKey: req.IdempotencyKey,
		RequestHash:    req.RequestHash,
		ResponseStatus: req.ResponseStatus,
		ResponseBody:   req.ResponseBody,
		CreatedAt:      now,
		ExpiresAt:      now.Add(s.ttl),
	}
	if err := rec.Validate(); err != nil {
		return ResolveResult{}, internalerrors.New(internalerrors.ErrValidationFailed, "validate idempotency record", err)
	}
	if err := s.repo.Create(ctx, rec); err != nil {
		return ResolveResult{}, internalerrors.New(internalerrors.ErrDatabaseError, "create idempotency record", err)
	}

	return ResolveResult{Replayed: false, ResponseStatus: req.ResponseStatus, ResponseBody: req.ResponseBody}, nil
}

func (s *Service) DeleteExpired(ctx context.Context, now time.Time) (int64, error) {
	if s.repo == nil {
		return 0, internalerrors.New(internalerrors.ErrInternalError, "idempotency repository is required", nil)
	}
	if now.IsZero() {
		return 0, internalerrors.New(internalerrors.ErrValidationFailed, "now is required", nil)
	}
	deleted, err := s.repo.DeleteExpired(ctx, now.UTC())
	if err != nil {
		return 0, internalerrors.New(internalerrors.ErrDatabaseError, "delete expired idempotency records", err)
	}
	return deleted, nil
}

func generateID(r io.Reader) (string, error) {
	b := make([]byte, 8)
	if _, err := io.ReadFull(r, b); err != nil {
		return "", err
	}
	return "idem_" + hex.EncodeToString(b), nil
}
