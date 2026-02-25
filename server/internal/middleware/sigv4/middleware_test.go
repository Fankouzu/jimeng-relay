package sigv4

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/jimeng-relay/server/internal/models"
	"github.com/jimeng-relay/server/internal/repository"
	"github.com/jimeng-relay/server/internal/secretcrypto"
	apikeyservice "github.com/jimeng-relay/server/internal/service/apikey"
)

func TestMiddleware_ValidSignaturePasses(t *testing.T) {
	now := time.Date(2026, 2, 24, 10, 0, 0, 0, time.UTC)
	c := mustTestCipher(t)
	repo := &stubRepo{key: activeKey(t, c, "key_1", "ak_test", "sk_test_secret")}
	mw := New(repo, Config{Now: func() time.Time { return now }, SecretCipher: c})

	body := []byte(`{"prompt":"cat"}`)
	req := newSignedRequest(t, http.MethodPost, "http://relay.local/v1/submit", body, "ak_test", "sk_test_secret", now)
	rec := httptest.NewRecorder()

	passed := false
	mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		passed = true
		if got := r.Context().Value(ContextAPIKeyID); got != "key_1" {
			t.Fatalf("unexpected api key id in context: %#v", got)
		}
		w.WriteHeader(http.StatusOK)
	})).ServeHTTP(rec, req)

	if !passed {
		t.Fatalf("expected request to pass middleware")
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestMiddleware_TamperedBodyRejected(t *testing.T) {
	now := time.Date(2026, 2, 24, 10, 0, 0, 0, time.UTC)
	c := mustTestCipher(t)
	repo := &stubRepo{key: activeKey(t, c, "key_1", "ak_test", "sk_test_secret")}
	mw := New(repo, Config{Now: func() time.Time { return now }, SecretCipher: c})

	req := newSignedRequest(t, http.MethodPost, "http://relay.local/v1/submit", []byte(`{"prompt":"cat"}`), "ak_test", "sk_test_secret", now)
	req.Body = io.NopCloser(bytes.NewReader([]byte(`{"prompt":"dog"}`)))
	rec := httptest.NewRecorder()

	mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})).ServeHTTP(rec, req)

	assertErrorCode(t, rec, http.StatusUnauthorized, "INVALID_SIGNATURE")
}

func TestMiddleware_SignatureMismatchRejected(t *testing.T) {
	now := time.Date(2026, 2, 24, 10, 0, 0, 0, time.UTC)
	c := mustTestCipher(t)
	repo := &stubRepo{key: activeKey(t, c, "key_1", "ak_test", "sk_test_secret")}
	mw := New(repo, Config{Now: func() time.Time { return now }, SecretCipher: c})

	req := newSignedRequest(t, http.MethodPost, "http://relay.local/v1/submit", []byte(`{"prompt":"cat"}`), "ak_test", "sk_test_secret", now)
	auth := req.Header.Get("Authorization")
	if auth == "" {
		t.Fatalf("expected authorization header")
	}
	if strings.HasSuffix(auth, "0") {
		auth = strings.TrimSuffix(auth, "0") + "1"
	} else {
		auth = strings.TrimSuffix(auth, auth[len(auth)-1:]) + "0"
	}
	req.Header.Set("Authorization", auth)

	rec := httptest.NewRecorder()
	mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})).ServeHTTP(rec, req)

	assertErrorCode(t, rec, http.StatusUnauthorized, "INVALID_SIGNATURE")
}

func TestMiddleware_ExpiredXDateRejected(t *testing.T) {
	now := time.Date(2026, 2, 24, 10, 0, 0, 0, time.UTC)
	c := mustTestCipher(t)
	repo := &stubRepo{key: activeKey(t, c, "key_1", "ak_test", "sk_test_secret")}
	mw := New(repo, Config{Now: func() time.Time { return now }, ClockSkew: 5 * time.Minute, SecretCipher: c})

	old := now.Add(-5*time.Minute - time.Second)
	req := newSignedRequest(t, http.MethodPost, "http://relay.local/v1/submit", []byte(`{"prompt":"cat"}`), "ak_test", "sk_test_secret", old)
	rec := httptest.NewRecorder()

	mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})).ServeHTTP(rec, req)

	assertErrorCode(t, rec, http.StatusUnauthorized, "AUTH_FAILED")
}

func TestMiddleware_FutureXDateRejected(t *testing.T) {
	now := time.Date(2026, 2, 24, 10, 0, 0, 0, time.UTC)
	c := mustTestCipher(t)
	repo := &stubRepo{key: activeKey(t, c, "key_1", "ak_test", "sk_test_secret")}
	mw := New(repo, Config{Now: func() time.Time { return now }, ClockSkew: 5 * time.Minute, SecretCipher: c})

	future := now.Add(5*time.Minute + time.Second)
	req := newSignedRequest(t, http.MethodPost, "http://relay.local/v1/submit", []byte(`{"prompt":"cat"}`), "ak_test", "sk_test_secret", future)
	rec := httptest.NewRecorder()

	mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})).ServeHTTP(rec, req)

	assertErrorCode(t, rec, http.StatusUnauthorized, "AUTH_FAILED")
}

func TestMiddleware_RevokedOrExpiredKeyRejected(t *testing.T) {
	now := time.Date(2026, 2, 24, 10, 0, 0, 0, time.UTC)
	revokedAt := now.Add(-time.Minute)
	expiresAt := now.Add(-time.Minute)
	c := mustTestCipher(t)
	active := activeKey(t, c, "key_base", "ak_test", "sk_test_secret")

	tests := []struct {
		name   string
		key    models.APIKey
		expect string
	}{
		{
			name:   "revoked",
			key:    withRevoked(active, "key_r", &revokedAt),
			expect: "KEY_REVOKED",
		},
		{
			name:   "revoked_at_only",
			key:    withRevokedAtOnly(active, "key_r2", &revokedAt),
			expect: "KEY_REVOKED",
		},
		{
			name:   "expired",
			key:    withExpired(active, "key_e", &expiresAt),
			expect: "KEY_EXPIRED",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &stubRepo{key: tt.key}
			mw := New(repo, Config{Now: func() time.Time { return now }, SecretCipher: c})
			req := newSignedRequest(t, http.MethodPost, "http://relay.local/v1/submit", []byte(`{"prompt":"cat"}`), "ak_test", "sk_test_secret", now)
			rec := httptest.NewRecorder()

			mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
			})).ServeHTTP(rec, req)

			assertErrorCode(t, rec, http.StatusUnauthorized, tt.expect)
		})
	}
}

func TestMiddleware_LegacyManagementPathsNoLongerBypassVerification(t *testing.T) {
	now := time.Date(2026, 2, 24, 10, 0, 0, 0, time.UTC)
	mw := New(&stubRepo{err: repository.ErrNotFound}, Config{Now: func() time.Time { return now }, SecretCipher: mustTestCipher(t)})

	paths := []string{"/v1/keys", "/v1/keys/abc/revoke", "/v1/keys/abc/rotate"}
	for _, p := range paths {
		t.Run(p, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "http://relay.local"+p, nil)
			rec := httptest.NewRecorder()
			mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusNoContent)
			})).ServeHTTP(rec, req)
			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("expected auth required for legacy management route %s, got %d", p, rec.Code)
			}
		})
	}
}

func TestMiddleware_EndToEnd_CreateKeyThenSignAccepted(t *testing.T) {
	now := time.Date(2026, 2, 24, 10, 0, 0, 0, time.UTC)
	repo := &memoryAPIKeyRepo{keys: map[string]models.APIKey{}}
	c := mustTestCipher(t)
	svc := apikeyservice.NewService(repo, apikeyservice.Config{
		Now:          func() time.Time { return now },
		BcryptCost:   4,
		SecretCipher: c,
	})
	created, err := svc.Create(context.Background(), apikeyservice.CreateRequest{Description: "e2e"})
	if err != nil {
		t.Fatalf("create key: %v", err)
	}
	if created.SecretKey == "" {
		t.Fatalf("expected created secret key")
	}

	mw := New(repo, Config{Now: func() time.Time { return now }, SecretCipher: c})
	req := newSignedRequest(t, http.MethodPost, "http://relay.local/v1/submit", []byte(`{"prompt":"cat"}`), created.AccessKey, created.SecretKey, now)
	rec := httptest.NewRecorder()

	mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})).ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("unexpected status: %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestMiddleware_CredentialScopeServiceMismatchRejected(t *testing.T) {
	now := time.Date(2026, 2, 24, 10, 0, 0, 0, time.UTC)
	c := mustTestCipher(t)
	repo := &stubRepo{key: activeKey(t, c, "key_1", "ak_test", "sk_test_secret")}
	mw := New(repo, Config{Now: func() time.Time { return now }, SecretCipher: c})

	req := newSignedRequestWithCredentialScope(t, http.MethodPost, "http://relay.local/v1/submit", []byte(`{"prompt":"cat"}`), "ak_test", "sk_test_secret", now, now.UTC().Format("20060102"), "cn-north-1", "evil")
	rec := httptest.NewRecorder()

	mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})).ServeHTTP(rec, req)

	assertErrorCode(t, rec, http.StatusUnauthorized, "AUTH_FAILED")
}

func TestMiddleware_CredentialScopeDateMismatchRejected(t *testing.T) {
	now := time.Date(2026, 2, 24, 10, 0, 0, 0, time.UTC)
	c := mustTestCipher(t)
	repo := &stubRepo{key: activeKey(t, c, "key_1", "ak_test", "sk_test_secret")}
	mw := New(repo, Config{Now: func() time.Time { return now }, SecretCipher: c})

	wrongDateScope := now.Add(-24 * time.Hour).UTC().Format("20060102")
	req := newSignedRequestWithCredentialScope(t, http.MethodPost, "http://relay.local/v1/submit", []byte(`{"prompt":"cat"}`), "ak_test", "sk_test_secret", now, wrongDateScope, "cn-north-1", "cv")
	rec := httptest.NewRecorder()

	mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})).ServeHTTP(rec, req)

	assertErrorCode(t, rec, http.StatusUnauthorized, "AUTH_FAILED")
}

type stubRepo struct {
	key models.APIKey
	err error
}

func (s *stubRepo) Create(context.Context, models.APIKey) error { return nil }
func (s *stubRepo) GetByID(context.Context, string) (models.APIKey, error) {
	return models.APIKey{}, repository.ErrNotFound
}
func (s *stubRepo) GetByAccessKey(context.Context, string) (models.APIKey, error) {
	if s.err != nil {
		return models.APIKey{}, s.err
	}
	return s.key, nil
}
func (s *stubRepo) List(context.Context) ([]models.APIKey, error)         { return nil, nil }
func (s *stubRepo) Revoke(context.Context, string, time.Time) error       { return nil }
func (s *stubRepo) SetExpired(context.Context, string, time.Time) error   { return nil }
func (s *stubRepo) SetExpiresAt(context.Context, string, time.Time) error { return nil }

type memoryAPIKeyRepo struct {
	keys map[string]models.APIKey
}

func (m *memoryAPIKeyRepo) Create(_ context.Context, key models.APIKey) error {
	m.keys[key.ID] = key
	return nil
}

func (m *memoryAPIKeyRepo) GetByID(_ context.Context, id string) (models.APIKey, error) {
	key, ok := m.keys[id]
	if !ok {
		return models.APIKey{}, repository.ErrNotFound
	}
	return key, nil
}

func (m *memoryAPIKeyRepo) GetByAccessKey(_ context.Context, accessKey string) (models.APIKey, error) {
	for _, key := range m.keys {
		if key.AccessKey == accessKey {
			return key, nil
		}
	}
	return models.APIKey{}, repository.ErrNotFound
}

func (m *memoryAPIKeyRepo) List(_ context.Context) ([]models.APIKey, error) {
	out := make([]models.APIKey, 0, len(m.keys))
	for _, k := range m.keys {
		out = append(out, k)
	}
	return out, nil
}

func (m *memoryAPIKeyRepo) Revoke(_ context.Context, id string, revokedAt time.Time) error {
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

func (m *memoryAPIKeyRepo) SetExpired(_ context.Context, id string, expiredAt time.Time) error {
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

func (m *memoryAPIKeyRepo) SetExpiresAt(_ context.Context, id string, expiresAt time.Time) error {
	key, ok := m.keys[id]
	if !ok {
		return repository.ErrNotFound
	}
	key.ExpiresAt = &expiresAt
	key.UpdatedAt = expiresAt
	m.keys[id] = key
	return nil
}

func mustTestCipher(t *testing.T) secretcrypto.Cipher {
	t.Helper()
	c, err := secretcrypto.NewAESCipher([]byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatalf("NewAESCipher: %v", err)
	}
	return c
}

func activeKey(t *testing.T, c secretcrypto.Cipher, id, ak, sk string) models.APIKey {
	t.Helper()
	ct, err := c.Encrypt(sk)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	return models.APIKey{
		ID:                  id,
		AccessKey:           ak,
		SecretKeyHash:       "bcrypt-placeholder",
		SecretKeyCiphertext: ct,
		Status:              models.APIKeyStatusActive,
		CreatedAt:           time.Date(2026, 2, 24, 10, 0, 0, 0, time.UTC),
		UpdatedAt:           time.Date(2026, 2, 24, 10, 0, 0, 0, time.UTC),
	}
}

func withRevoked(base models.APIKey, id string, revokedAt *time.Time) models.APIKey {
	b := base
	b.ID = id
	b.RevokedAt = revokedAt
	b.Status = models.APIKeyStatusRevoked
	return b
}

func withRevokedAtOnly(base models.APIKey, id string, revokedAt *time.Time) models.APIKey {
	b := base
	b.ID = id
	b.RevokedAt = revokedAt
	b.Status = models.APIKeyStatusActive
	return b
}

func withExpired(base models.APIKey, id string, expiresAt *time.Time) models.APIKey {
	b := base
	b.ID = id
	b.ExpiresAt = expiresAt
	b.Status = models.APIKeyStatusActive
	return b
}

func assertErrorCode(t *testing.T, rec *httptest.ResponseRecorder, expectedStatus int, expectedCode string) {
	t.Helper()
	if rec.Code != expectedStatus {
		t.Fatalf("unexpected status: %d body=%s", rec.Code, rec.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal error body: %v body=%s", err, rec.Body.String())
	}
	errObj, ok := payload["error"].(map[string]any)
	if !ok {
		t.Fatalf("error payload missing: %#v", payload)
	}
	if errObj["code"] != expectedCode {
		t.Fatalf("unexpected error code: got=%v want=%s body=%s", errObj["code"], expectedCode, rec.Body.String())
	}
}

func newSignedRequest(t *testing.T, method, target string, body []byte, accessKey, secret string, ts time.Time) *http.Request {
	return newSignedRequestWithCredentialScope(t, method, target, body, accessKey, secret, ts, ts.UTC().Format("20060102"), "cn-north-1", "cv")
}

func newSignedRequestWithCredentialScope(t *testing.T, method, target string, body []byte, accessKey, secret string, ts time.Time, credentialDateScope, credentialRegion, credentialService string) *http.Request {
	t.Helper()
	parsed, err := url.Parse(target)
	if err != nil {
		t.Fatalf("parse target: %v", err)
	}
	req := httptest.NewRequest(method, parsed.String(), bytes.NewReader(body))
	req.Host = parsed.Host

	date := ts.UTC().Format("20060102T150405Z")
	payloadHash := sha256Hex(body)
	req.Header.Set("X-Date", date)
	req.Header.Set("X-Content-Sha256", payloadHash)

	signedHeaders := []string{"host", "x-content-sha256", "x-date"}
	canon, err := buildCanonicalRequest(req, signedHeaders, payloadHash)
	if err != nil {
		t.Fatalf("build canonical request: %v", err)
	}
	dateShort := strings.TrimSpace(credentialDateScope)
	if dateShort == "" {
		dateShort = ts.UTC().Format("20060102")
	}
	region := strings.TrimSpace(credentialRegion)
	if region == "" {
		region = "cn-north-1"
	}
	service := strings.TrimSpace(credentialService)
	if service == "" {
		service = "cv"
	}
	scope := dateShort + "/" + region + "/" + service + "/aws4_request"
	stringToSign := strings.Join([]string{
		"AWS4-HMAC-SHA256",
		date,
		scope,
		sha256Hex([]byte(canon)),
	}, "\n")

	signingKey := deriveSigningKey(secret, dateShort, region, service, "aws4_request")
	signature := hex.EncodeToString(hmacSHA256(signingKey, stringToSign))
	req.Header.Set("Authorization", "AWS4-HMAC-SHA256 Credential="+accessKey+"/"+scope+", SignedHeaders="+strings.Join(signedHeaders, ";")+", Signature="+signature)
	return req
}
