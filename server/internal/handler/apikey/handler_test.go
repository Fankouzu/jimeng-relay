package apikey

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jimeng-relay/server/internal/models"
	"github.com/jimeng-relay/server/internal/repository"
	"github.com/jimeng-relay/server/internal/secretcrypto"
	service "github.com/jimeng-relay/server/internal/service/apikey"
)

type memoryRepo struct {
	keys map[string]models.APIKey
}

func newMemoryRepo() *memoryRepo {
	return &memoryRepo{keys: map[string]models.APIKey{}}
}

func (m *memoryRepo) Create(_ context.Context, key models.APIKey) error {
	if _, exists := m.keys[key.ID]; exists {
		return errors.New("duplicate id")
	}
	for _, existing := range m.keys {
		if existing.AccessKey == key.AccessKey {
			return errors.New("duplicate access key")
		}
	}
	m.keys[key.ID] = key
	return nil
}

func (m *memoryRepo) GetByID(_ context.Context, id string) (models.APIKey, error) {
	key, ok := m.keys[id]
	if !ok {
		return models.APIKey{}, repository.ErrNotFound
	}
	return key, nil
}

func (m *memoryRepo) GetByAccessKey(_ context.Context, accessKey string) (models.APIKey, error) {
	for _, key := range m.keys {
		if key.AccessKey == accessKey {
			return key, nil
		}
	}
	return models.APIKey{}, repository.ErrNotFound
}

func (m *memoryRepo) List(_ context.Context) ([]models.APIKey, error) {
	out := make([]models.APIKey, 0, len(m.keys))
	for _, key := range m.keys {
		out = append(out, key)
	}
	return out, nil
}

func (m *memoryRepo) Revoke(_ context.Context, id string, revokedAt time.Time) error {
	key, ok := m.keys[id]
	if !ok {
		return repository.ErrNotFound
	}
	key.RevokedAt = &revokedAt
	key.Status = models.APIKeyStatusRevoked
	key.UpdatedAt = revokedAt
	m.keys[id] = key
	return nil
}

func (m *memoryRepo) SetExpired(_ context.Context, id string, expiredAt time.Time) error {
	key, ok := m.keys[id]
	if !ok {
		return repository.ErrNotFound
	}
	key.ExpiresAt = &expiredAt
	key.Status = models.APIKeyStatusExpired
	key.UpdatedAt = expiredAt
	m.keys[id] = key
	return nil
}

func (m *memoryRepo) SetExpiresAt(_ context.Context, id string, expiresAt time.Time) error {
	key, ok := m.keys[id]
	if !ok {
		return repository.ErrNotFound
	}
	key.ExpiresAt = &expiresAt
	key.UpdatedAt = expiresAt
	m.keys[id] = key
	return nil
}

func TestHandler_CreateListRevokeRotateLifecycle(t *testing.T) {
	base := time.Date(2026, 2, 24, 10, 0, 0, 0, time.UTC)
	repo := newMemoryRepo()
	svc := service.NewService(repo, service.Config{
		Now:          func() time.Time { return base },
		BcryptCost:   4,
		SecretCipher: mustTestCipher(t),
	})
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	h := NewHandler(svc, logger)

	createReq := map[string]any{"description": "http-key"}
	createdBody := doJSONRequest(t, h.Routes(), http.MethodPost, "/v1/keys", createReq, http.StatusCreated)
	if createdBody["secret_key"] == "" {
		t.Fatalf("expected secret_key in create response")
	}
	createdID, ok := createdBody["id"].(string)
	if !ok {
		t.Fatalf("expected id to be string, got %#v", createdBody["id"])
	}
	if createdID == "" {
		t.Fatalf("expected id in create response")
	}

	listBody := doJSONRequest(t, h.Routes(), http.MethodGet, "/v1/keys", nil, http.StatusOK)
	items, ok := listBody["items"].([]any)
	if !ok || len(items) == 0 {
		t.Fatalf("expected non-empty items")
	}
	first, ok := items[0].(map[string]any)
	if !ok {
		t.Fatalf("unexpected list item payload")
	}
	if _, exists := first["secret_key"]; exists {
		t.Fatalf("list response must not include plaintext secret")
	}
	if _, exists := first["secret_key_hash"]; exists {
		t.Fatalf("list response must not include secret hash")
	}

	doJSONRequest(t, h.Routes(), http.MethodPost, "/v1/keys/"+createdID+"/revoke", map[string]any{}, http.StatusOK)

	created2 := doJSONRequest(t, h.Routes(), http.MethodPost, "/v1/keys", map[string]any{"description": "for rotate"}, http.StatusCreated)
	created2ID, ok := created2["id"].(string)
	if !ok {
		t.Fatalf("expected second id to be string, got %#v", created2["id"])
	}

	rotated := doJSONRequest(t, h.Routes(), http.MethodPost, "/v1/keys/"+created2ID+"/rotate", map[string]any{"grace_period_seconds": 300}, http.StatusCreated)
	if rotated["rotation_of"] != created2ID {
		t.Fatalf("expected rotation_of=%q, got %#v", created2ID, rotated["rotation_of"])
	}
	if rotated["secret_key"] == "" {
		t.Fatalf("expected plaintext secret for rotated key")
	}
	old, err := repo.GetByID(context.TODO(), created2ID)
	if err != nil {
		t.Fatalf("GetByID old key: %v", err)
	}
	if old.ExpiresAt == nil {
		t.Fatalf("expected old key expires_at to be set by rotation window")
	}
}

func mustTestCipher(t *testing.T) secretcrypto.Cipher {
	t.Helper()
	c, err := secretcrypto.NewAESCipher([]byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatalf("NewAESCipher: %v", err)
	}
	return c
}

func doJSONRequest(t *testing.T, handler http.Handler, method, path string, payload any, expectedStatus int) map[string]any {
	t.Helper()

	var body io.Reader
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			t.Fatalf("Marshal payload: %v", err)
		}
		body = bytes.NewReader(b)
	}
	req := httptest.NewRequest(method, path, body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != expectedStatus {
		t.Fatalf("unexpected status %d body=%s", rec.Code, rec.Body.String())
	}

	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("Unmarshal response: %v body=%s", err, rec.Body.String())
	}
	return out
}
