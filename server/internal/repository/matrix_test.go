package repository_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jimeng-relay/server/internal/models"
	"github.com/jimeng-relay/server/internal/repository"
	"github.com/jimeng-relay/server/internal/repository/postgres"
	"github.com/jimeng-relay/server/internal/repository/sqlite"
)

type matrixRepos struct {
	APIKeys            repository.APIKeyRepository
	AuditEvents        repository.AuditEventRepository
	IdempotencyRecords repository.IdempotencyRecordRepository
}

type matrixBackend struct {
	Name string
	Open func(t *testing.T) (matrixRepos, func())
}

func TestRepositoryMatrix_SQLiteAndPostgres_Semantics(t *testing.T) {
	backends := []matrixBackend{
		{
			Name: "sqlite",
			Open: func(t *testing.T) (matrixRepos, func()) {
				dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", matrixID(t, "sqlite"))
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				t.Cleanup(cancel)

				repos, err := sqlite.Open(ctx, dsn)
				if err != nil {
					t.Fatalf("sqlite.Open: %v", err)
				}
				return matrixRepos{
					APIKeys:            repos.APIKeys,
					AuditEvents:        repos.AuditEvents,
					IdempotencyRecords: repos.IdempotencyRecords,
				}, func() { _ = repos.Close() }
			},
		},
		{
			Name: "postgres",
			Open: func(t *testing.T) (matrixRepos, func()) {
				dbURL := strings.TrimSpace(os.Getenv("DATABASE_URL"))
				if dbURL == "" {
					t.Skip("DATABASE_URL not set")
				}
				ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
				t.Cleanup(cancel)

				db, err := postgres.Open(ctx, dbURL)
				if err != nil {
					t.Fatalf("postgres.Open: %v", err)
				}
				return matrixRepos{
					APIKeys:            db.APIKeys(),
					AuditEvents:        db.AuditEvents(),
					IdempotencyRecords: db.IdempotencyRecords(),
				}, func() { db.Close() }
			},
		},
	}

	for _, b := range backends {
		b := b
		t.Run(b.Name, func(t *testing.T) {
			repos, cleanup := b.Open(t)
			t.Cleanup(cleanup)

			testAPIKeyLifecycleSemantics(t, repos)
			testAuditEventSemantics(t, repos)
			testIdempotencySemantics(t, repos)
		})
	}
}

func testAPIKeyLifecycleSemantics(t *testing.T, repos matrixRepos) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)

	missingAccessKey := matrixID(t, "missing_ak")
	_, err := repos.APIKeys.GetByAccessKey(ctx, missingAccessKey)
	requireNotFound(t, err, "GetByAccessKey(missing)")
	_, err = repos.APIKeys.GetByID(ctx, matrixID(t, "missing_id"))
	requireNotFound(t, err, "GetByID(missing)")
	err = repos.APIKeys.Revoke(ctx, matrixID(t, "missing_id"), time.Date(2400, 1, 1, 0, 0, 0, 0, time.UTC))
	requireNotFound(t, err, "Revoke(missing)")
	err = repos.APIKeys.SetExpired(ctx, matrixID(t, "missing_id"), time.Date(2400, 1, 1, 0, 0, 0, 0, time.UTC))
	requireNotFound(t, err, "SetExpired(missing)")
	err = repos.APIKeys.SetExpiresAt(ctx, matrixID(t, "missing_id"), time.Date(2400, 1, 1, 0, 0, 0, 0, time.UTC))
	requireNotFound(t, err, "SetExpiresAt(missing)")

	base := time.Date(2400, 2, 24, 3, 4, 5, 123000000, time.UTC).Truncate(time.Microsecond)
	accessKey1 := matrixID(t, "ak1")
	accessKey2 := matrixID(t, "ak2")
	key1 := models.APIKey{
		ID:                  matrixID(t, "k1"),
		AccessKey:           accessKey1,
		SecretKeyHash:       "$2a$10$abcdefghijklmnopqrstuv",
		SecretKeyCiphertext: "v1:test",
		Description:         "matrix",
		Status:              models.APIKeyStatusActive,
		CreatedAt:           base,
		UpdatedAt:           base,
	}
	if err := repos.APIKeys.Create(ctx, key1); err != nil {
		t.Fatalf("Create(k1): %v", err)
	}

	expiresAt := base.Add(24 * time.Hour)
	key2 := models.APIKey{
		ID:                  matrixID(t, "k2"),
		AccessKey:           accessKey2,
		SecretKeyHash:       "$2a$10$abcdefghijklmnopqrstuv",
		SecretKeyCiphertext: "v1:test2",
		Description:         "matrix2",
		Status:              models.APIKeyStatusActive,
		CreatedAt:           base.Add(time.Second),
		UpdatedAt:           base.Add(time.Second),
		ExpiresAt:           &expiresAt,
	}
	if err := repos.APIKeys.Create(ctx, key2); err != nil {
		t.Fatalf("Create(k2): %v", err)
	}

	fetched1, err := repos.APIKeys.GetByAccessKey(ctx, accessKey1)
	if err != nil {
		t.Fatalf("GetByAccessKey(k1): %v", err)
	}
	if fetched1.ID != key1.ID || fetched1.AccessKey != key1.AccessKey || fetched1.SecretKeyCiphertext != key1.SecretKeyCiphertext {
		t.Fatalf("unexpected fetched1: %#v", fetched1)
	}

	fetched2, err := repos.APIKeys.GetByID(ctx, key2.ID)
	if err != nil {
		t.Fatalf("GetByID(k2): %v", err)
	}
	if fetched2.ExpiresAt == nil || !fetched2.ExpiresAt.Equal(expiresAt) {
		t.Fatalf("expected expires_at to roundtrip")
	}

	newExpiresAt := base.Add(10 * time.Minute)
	if err := repos.APIKeys.SetExpiresAt(ctx, key1.ID, newExpiresAt); err != nil {
		t.Fatalf("SetExpiresAt(k1): %v", err)
	}
	fetched1, err = repos.APIKeys.GetByID(ctx, key1.ID)
	if err != nil {
		t.Fatalf("GetByID(k1 after SetExpiresAt): %v", err)
	}
	if fetched1.ExpiresAt == nil || !fetched1.ExpiresAt.Equal(newExpiresAt) {
		t.Fatalf("expected expires_at to be set")
	}
	if fetched1.Status != models.APIKeyStatusActive {
		t.Fatalf("SetExpiresAt should keep status active, got %q", fetched1.Status)
	}

	expiredAt := base.Add(2 * time.Hour)
	if err := repos.APIKeys.SetExpired(ctx, key2.ID, expiredAt); err != nil {
		t.Fatalf("SetExpired(k2): %v", err)
	}
	fetched2, err = repos.APIKeys.GetByID(ctx, key2.ID)
	if err != nil {
		t.Fatalf("GetByID(k2 after SetExpired): %v", err)
	}
	if fetched2.Status != models.APIKeyStatusExpired {
		t.Fatalf("expected status expired, got %q", fetched2.Status)
	}
	if fetched2.ExpiresAt == nil || !fetched2.ExpiresAt.Equal(expiredAt) {
		t.Fatalf("expected expires_at to be set by SetExpired")
	}

	revokedAt := base.Add(time.Minute)
	if err := repos.APIKeys.Revoke(ctx, key1.ID, revokedAt); err != nil {
		t.Fatalf("Revoke(k1): %v", err)
	}
	fetched1, err = repos.APIKeys.GetByID(ctx, key1.ID)
	if err != nil {
		t.Fatalf("GetByID(k1 after Revoke): %v", err)
	}
	if fetched1.Status != models.APIKeyStatusRevoked {
		t.Fatalf("expected status revoked, got %q", fetched1.Status)
	}
	if fetched1.RevokedAt == nil || !fetched1.RevokedAt.Equal(revokedAt) {
		t.Fatalf("expected revoked_at to be set")
	}

	if err := repos.APIKeys.SetExpired(ctx, key1.ID, base.Add(3*time.Hour)); err != nil {
		t.Fatalf("SetExpired(revoked k1): %v", err)
	}
	fetched1, err = repos.APIKeys.GetByID(ctx, key1.ID)
	if err != nil {
		t.Fatalf("GetByID(k1 after SetExpired while revoked): %v", err)
	}
	if fetched1.Status != models.APIKeyStatusRevoked {
		t.Fatalf("expected revoked status to remain, got %q", fetched1.Status)
	}

	list, err := repos.APIKeys.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	idx1 := indexAPIKeyByID(list, key1.ID)
	idx2 := indexAPIKeyByID(list, key2.ID)
	if idx1 < 0 || idx2 < 0 {
		t.Fatalf("expected both keys present in List")
	}
	if idx2 >= idx1 {
		t.Fatalf("expected newer key (k2) to be listed before older key (k1)")
	}
}

func testAuditEventSemantics(t *testing.T, repos matrixRepos) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)

	base := time.Date(2400, 2, 24, 3, 4, 5, 0, time.UTC).Truncate(time.Microsecond)
	req1 := matrixID(t, "req1")
	req2 := matrixID(t, "req2")

	e1 := models.AuditEvent{
		ID:        matrixID(t, "a1"),
		RequestID: req1,
		EventType: models.EventTypeRequestReceived,
		Actor:     "system",
		Action:    "received",
		Resource:  "relay.request",
		Metadata:  map[string]any{"m": "1"},
		CreatedAt: base,
	}
	if err := repos.AuditEvents.Create(ctx, e1); err != nil {
		t.Fatalf("Create(a1): %v", err)
	}
	e2 := models.AuditEvent{
		ID:        matrixID(t, "a2"),
		RequestID: req1,
		EventType: models.EventTypeResponseSent,
		Actor:     "system",
		Action:    "sent",
		Resource:  "relay.response",
		CreatedAt: base.Add(2 * time.Second),
	}
	if err := repos.AuditEvents.Create(ctx, e2); err != nil {
		t.Fatalf("Create(a2): %v", err)
	}
	e3 := models.AuditEvent{
		ID:        matrixID(t, "a3"),
		RequestID: req2,
		EventType: models.EventTypeError,
		Actor:     "system",
		Action:    "error",
		Resource:  "relay.error",
		CreatedAt: base.Add(3 * time.Second),
	}
	if err := repos.AuditEvents.Create(ctx, e3); err != nil {
		t.Fatalf("Create(a3): %v", err)
	}

	byReq, err := repos.AuditEvents.ListByRequestID(ctx, req1)
	if err != nil {
		t.Fatalf("ListByRequestID: %v", err)
	}
	if len(byReq) < 2 {
		t.Fatalf("expected at least 2 events for req1, got %d", len(byReq))
	}
	idxE1 := indexAuditEventByID(byReq, e1.ID)
	idxE2 := indexAuditEventByID(byReq, e2.ID)
	if idxE1 < 0 || idxE2 < 0 {
		t.Fatalf("expected a1 and a2 present in ListByRequestID")
	}
	if idxE1 >= idxE2 {
		t.Fatalf("expected ordering by created_at ASC for req1")
	}
	if byReq[idxE1].Metadata["m"] != "1" {
		t.Fatalf("expected metadata to roundtrip")
	}

	start := base.Add(time.Second)
	end := base.Add(3 * time.Second)
	byTime, err := repos.AuditEvents.ListByTimeRange(ctx, start, end)
	if err != nil {
		t.Fatalf("ListByTimeRange: %v", err)
	}
	idxE2 = indexAuditEventByID(byTime, e2.ID)
	idxE3 := indexAuditEventByID(byTime, e3.ID)
	if idxE2 < 0 || idxE3 < 0 {
		t.Fatalf("expected a2 and a3 present in ListByTimeRange")
	}
	if idxE2 >= idxE3 {
		t.Fatalf("expected ordering by created_at ASC in time range")
	}
}

func testIdempotencySemantics(t *testing.T, repos matrixRepos) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)

	missingKey := matrixID(t, "missing_idem")
	_, err := repos.IdempotencyRecords.GetByKey(ctx, missingKey)
	requireNotFound(t, err, "GetByKey(missing)")

	now := time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC)
	keyLive := matrixID(t, "idem_live")
	keyExpired := matrixID(t, "idem_expired")

	live := models.IdempotencyRecord{
		ID:             matrixID(t, "i_live"),
		IdempotencyKey: keyLive,
		RequestHash:    "h_live",
		ResponseStatus: 200,
		ResponseBody:   map[string]any{"task_id": "t1"},
		CreatedAt:      now.Add(-time.Hour),
		ExpiresAt:      now.Add(time.Hour),
	}
	if err := repos.IdempotencyRecords.Create(ctx, live); err != nil {
		t.Fatalf("Create(live): %v", err)
	}
	expired := models.IdempotencyRecord{
		ID:             matrixID(t, "i_expired"),
		IdempotencyKey: keyExpired,
		RequestHash:    "h_expired",
		ResponseStatus: 200,
		CreatedAt:      now.Add(-2 * time.Hour),
		ExpiresAt:      now.Add(-time.Minute),
	}
	if err := repos.IdempotencyRecords.Create(ctx, expired); err != nil {
		t.Fatalf("Create(expired): %v", err)
	}

	fetched, err := repos.IdempotencyRecords.GetByKey(ctx, keyLive)
	if err != nil {
		t.Fatalf("GetByKey(live): %v", err)
	}
	if fetched.RequestHash != "h_live" || fetched.ResponseStatus != 200 {
		t.Fatalf("unexpected record: %#v", fetched)
	}
	if body, ok := fetched.ResponseBody.(map[string]any); !ok || body["task_id"] != "t1" {
		t.Fatalf("expected response body to roundtrip")
	}

	if _, err := repos.IdempotencyRecords.DeleteExpired(ctx, now); err != nil {
		t.Fatalf("DeleteExpired: %v", err)
	}
	_, err = repos.IdempotencyRecords.GetByKey(ctx, keyExpired)
	requireNotFound(t, err, "GetByKey(expired after DeleteExpired)")
	if _, err := repos.IdempotencyRecords.GetByKey(ctx, keyLive); err != nil {
		t.Fatalf("expected live record to remain: %v", err)
	}

	dup := live
	dup.ID = matrixID(t, "i_dup")
	dup.RequestHash = "h_dup"
	if err := repos.IdempotencyRecords.Create(ctx, dup); err == nil {
		t.Fatalf("expected unique constraint error on duplicate idempotency_key")
	}
}

func matrixID(t *testing.T, prefix string) string {
	t.Helper()

	n := strings.NewReplacer("/", "_", " ", "_", ":", "_", "-", "_", ".", "_").Replace(t.Name())
	return prefix + "_" + n + "_" + strconv.FormatInt(time.Now().UnixNano(), 36)
}

func requireNotFound(t *testing.T, err error, op string) {
	t.Helper()
	if err == nil {
		t.Fatalf("%s: expected ErrNotFound, got nil", op)
	}
	if !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("%s: expected ErrNotFound, got %v", op, err)
	}
	if !repository.IsNotFound(err) {
		t.Fatalf("%s: expected IsNotFound=true, got false (%v)", op, err)
	}
}

func indexAPIKeyByID(list []models.APIKey, id string) int {
	for i := range list {
		if list[i].ID == id {
			return i
		}
	}
	return -1
}

func indexAuditEventByID(list []models.AuditEvent, id string) int {
	for i := range list {
		if list[i].ID == id {
			return i
		}
	}
	return -1
}
