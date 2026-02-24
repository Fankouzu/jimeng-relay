package sigv4

import (
	"bytes"
	"log"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	internalerrors "github.com/jimeng-relay/server/internal/errors"
	"github.com/jimeng-relay/server/internal/repository"
	"github.com/jimeng-relay/server/internal/secretcrypto"
)

const (
	defaultClockSkew = 5 * time.Minute
	xDateLayout      = "20060102T150405Z"
	maxSignedBodyBytes int64 = 2 << 20
)

type contextKey string

const ContextAPIKeyID contextKey = "api_key_id"

type Config struct {
	Now             func() time.Time
	ClockSkew       time.Duration
	SecretCipher    secretcrypto.Cipher
	ExpectedRegion  string
	ExpectedService string
}

type Middleware struct {
	repo            repository.APIKeyRepository
	now             func() time.Time
	clockSkew       time.Duration
	secretCipher    secretcrypto.Cipher
	expectedRegion  string
	expectedService string
}

func New(repo repository.APIKeyRepository, cfg Config) func(http.Handler) http.Handler {
	nowFn := cfg.Now
	if nowFn == nil {
		nowFn = func() time.Time { return time.Now().UTC() }
	}
	skew := cfg.ClockSkew
	if skew <= 0 {
		skew = defaultClockSkew
	}
	expectedRegion := strings.ToLower(strings.TrimSpace(cfg.ExpectedRegion))
	if expectedRegion == "" {
		expectedRegion = "cn-north-1"
	}
	expectedService := strings.ToLower(strings.TrimSpace(cfg.ExpectedService))
	if expectedService == "" {
		expectedService = "cv"
	}
	m := &Middleware{repo: repo, now: nowFn, clockSkew: skew, secretCipher: cfg.SecretCipher, expectedRegion: expectedRegion, expectedService: expectedService}
	return m.wrap
}

func (m *Middleware) wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, maxSignedBodyBytes)
		if err := m.verify(r); err != nil {
			writeUnauthorized(w, err)
			return
		}
		next.ServeHTTP(w, r)
	})
}

type authFields struct {
	accessKey     string
	dateScope     string
	region        string
	service       string
	signedHeaders []string
	signature     string
	scopeSuffix   string
}

func (m *Middleware) verify(r *http.Request) error {
	authorization := strings.TrimSpace(r.Header.Get("Authorization"))
	if authorization == "" {
		return internalerrors.New(internalerrors.ErrAuthFailed, "missing authorization header", nil)
	}
	fields, err := parseAuthorization(authorization)
	if err != nil {
		return internalerrors.New(internalerrors.ErrAuthFailed, "invalid authorization header", err)
	}
	if fields.accessKey == "" {
		return internalerrors.New(internalerrors.ErrAuthFailed, "missing access key in credential scope", nil)
	}

	xDate := strings.TrimSpace(r.Header.Get("X-Date"))
	if xDate == "" {
		return internalerrors.New(internalerrors.ErrAuthFailed, "missing x-date header", nil)
	}
	t, err := time.Parse(xDateLayout, xDate)
	if err != nil {
		return internalerrors.New(internalerrors.ErrAuthFailed, "invalid x-date header", err)
	}
	now := m.now().UTC()
	if t.UTC().Before(now.Add(-m.clockSkew)) || t.UTC().After(now.Add(m.clockSkew)) {
		return internalerrors.New(internalerrors.ErrAuthFailed, "request time is outside allowed window", nil)
	}
	dateShort := t.UTC().Format("20060102")
	if strings.TrimSpace(fields.dateScope) != dateShort {
		return internalerrors.New(internalerrors.ErrAuthFailed, "credential scope date does not match x-date", nil)
	}
	if strings.ToLower(strings.TrimSpace(fields.region)) != m.expectedRegion || strings.ToLower(strings.TrimSpace(fields.service)) != m.expectedService {
		return internalerrors.New(internalerrors.ErrAuthFailed, "credential scope region/service is not allowed", nil)
	}

	if !containsHeader(fields.signedHeaders, "host") || !containsHeader(fields.signedHeaders, "x-date") || !containsHeader(fields.signedHeaders, "x-content-sha256") {
		return internalerrors.New(internalerrors.ErrAuthFailed, "signed headers must include host, x-date and x-content-sha256", nil)
	}
	if strings.TrimSpace(r.Host) == "" {
		return internalerrors.New(internalerrors.ErrAuthFailed, "missing host header", nil)
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		return internalerrors.New(internalerrors.ErrAuthFailed, "read request body", err)
	}
	log.Printf("DEBUG: raw body: %s", string(body))
	r.Body = io.NopCloser(bytes.NewReader(body))
	payloadHash := sha256Hex(body)
	headerPayloadHash := strings.TrimSpace(r.Header.Get("X-Content-Sha256"))
	if headerPayloadHash == "" {
		return internalerrors.New(internalerrors.ErrAuthFailed, "missing x-content-sha256 header", nil)
	}
	if !constantTimeHexEqual(strings.ToLower(headerPayloadHash), payloadHash) {
		return internalerrors.New(internalerrors.ErrInvalidSignature, "payload hash mismatch", nil)
	}

	key, err := m.repo.GetByAccessKey(r.Context(), fields.accessKey)
	if err != nil {
		if repository.IsNotFound(err) {
			return internalerrors.New(internalerrors.ErrAuthFailed, "api key not found", err)
		}
		return internalerrors.New(internalerrors.ErrDatabaseError, "query api key", err)
	}
	if key.IsRevoked() {
		return internalerrors.New(internalerrors.ErrKeyRevoked, "api key is revoked", nil)
	}
	if key.Status == "expired" || (key.ExpiresAt != nil && !key.ExpiresAt.UTC().After(now)) {
		return internalerrors.New(internalerrors.ErrKeyExpired, "api key is expired", nil)
	}
	if m.secretCipher == nil {
		return internalerrors.New(internalerrors.ErrInternalError, "secret cipher is not configured", nil)
	}
	if strings.TrimSpace(key.SecretKeyCiphertext) == "" {
		return internalerrors.New(internalerrors.ErrAuthFailed, "api key secret is unavailable", nil)
	}
	secretKey, err := m.secretCipher.Decrypt(key.SecretKeyCiphertext)
	if err != nil {
		return internalerrors.New(internalerrors.ErrAuthFailed, "decrypt api key secret", err)
	}

	canonicalRequest, err := buildCanonicalRequest(r, fields.signedHeaders, payloadHash)
	if err != nil {
		return internalerrors.New(internalerrors.ErrAuthFailed, "build canonical request", err)
	}
	log.Printf("DEBUG: canonical request: %s", canonicalRequest)
	log.Printf("DEBUG: payload hash: %s", payloadHash)
	scope := strings.Join([]string{fields.dateScope, fields.region, fields.service, fields.scopeSuffix}, "/")
	// Use algorithm name based on scope suffix
	algorithm := "AWS4-HMAC-SHA256"
	if fields.scopeSuffix == "request" {
		algorithm = "HMAC-SHA256"
	}
	stringToSign := strings.Join([]string{
		algorithm,
		xDate,
		scope,
		sha256Hex([]byte(canonicalRequest)),
	}, "\n")
	signingKey := deriveSigningKey(secretKey, fields.dateScope, fields.region, fields.service, fields.scopeSuffix)
	log.Printf("DEBUG: stringToSign: %s", stringToSign)
	log.Printf("DEBUG: stringToSign hex: %x", stringToSign)
	log.Printf("DEBUG: signing key: %x", signingKey)
	expectedSignature := hex.EncodeToString(hmacSHA256(signingKey, stringToSign))
	// DEBUG: signature validation
	log.Printf("DEBUG: signature mismatch")
	log.Printf("DEBUG: expected signature: %s", strings.ToLower(expectedSignature))
	log.Printf("DEBUG: received signature: %s", strings.ToLower(fields.signature))
	log.Printf("DEBUG: scope suffix: %s", fields.scopeSuffix)
	log.Printf("DEBUG: algorithm: %s", algorithm)
	log.Printf("DEBUG: secret key (decrypted): %s", secretKey)
	log.Printf("DEBUG: date scope: %s", fields.dateScope)
	log.Printf("DEBUG: region: %s", fields.region)
	log.Printf("DEBUG: service: %s", fields.service)
	if !constantTimeHexEqual(fields.signature, expectedSignature) {
		return internalerrors.New(internalerrors.ErrInvalidSignature, "signature mismatch", nil)
	}

	ctx := context.WithValue(r.Context(), ContextAPIKeyID, key.ID)
	*r = *r.WithContext(ctx)
	return nil
}

func parseAuthorization(v string) (authFields, error) {
	var body string
	if strings.HasPrefix(v, "AWS4-HMAC-SHA256 ") {
		body = strings.TrimSpace(strings.TrimPrefix(v, "AWS4-HMAC-SHA256 "))
	} else if strings.HasPrefix(v, "HMAC-SHA256 ") {
		body = strings.TrimSpace(strings.TrimPrefix(v, "HMAC-SHA256 "))
	} else {
		return authFields{}, internalerrors.New(internalerrors.ErrAuthFailed, "unsupported authorization algorithm", nil)
	}
	parts := strings.Split(body, ",")
	values := map[string]string{}
	for _, part := range parts {
		kv := strings.SplitN(strings.TrimSpace(part), "=", 2)
		if len(kv) != 2 {
			return authFields{}, internalerrors.New(internalerrors.ErrAuthFailed, "invalid authorization kv segment", nil)
		}
		values[kv[0]] = kv[1]
	}
	credential := strings.TrimSpace(values["Credential"])
	signedHeaders := strings.TrimSpace(values["SignedHeaders"])
	signature := strings.TrimSpace(values["Signature"])
	if credential == "" || signedHeaders == "" || signature == "" {
		return authFields{}, internalerrors.New(internalerrors.ErrAuthFailed, "authorization fields are incomplete", nil)
	}
	scope := strings.Split(credential, "/")
	if len(scope) != 5 {
		return authFields{}, internalerrors.New(internalerrors.ErrAuthFailed, "invalid credential scope", nil)
	}
	// Support both "aws4_request" (AWS4) and "request" (Volc SDK) suffixes
	if scope[4] != "aws4_request" && scope[4] != "request" {
		return authFields{}, internalerrors.New(internalerrors.ErrAuthFailed, "invalid credential scope suffix", nil)
	}
	parsedHeaders := strings.Split(strings.ToLower(signedHeaders), ";")
	for i := range parsedHeaders {
		parsedHeaders[i] = strings.TrimSpace(parsedHeaders[i])
	}
	if scope[0] == "" || scope[1] == "" || scope[2] == "" || scope[3] == "" {
		return authFields{}, internalerrors.New(internalerrors.ErrAuthFailed, "credential scope is incomplete", nil)
	}
	return authFields{
		accessKey:     scope[0],
		dateScope:     scope[1],
		region:        scope[2],
		service:       scope[3],
		signedHeaders: parsedHeaders,
		signature:     strings.ToLower(signature),
		scopeSuffix:   scope[4],
	}, nil
}

func buildCanonicalRequest(r *http.Request, signedHeaders []string, payloadHash string) (string, error) {
	headers := append([]string(nil), signedHeaders...)
	sort.Strings(headers)
	canonHeaders := strings.Builder{}
	for _, h := range headers {
		if h == "" {
			return "", internalerrors.New(internalerrors.ErrAuthFailed, "empty signed header", nil)
		}
		v := canonicalHeaderValue(r, h)
		if v == "" {
			return "", internalerrors.New(internalerrors.ErrAuthFailed, "missing signed header: "+h, nil)
		}
		canonHeaders.WriteString(h)
		canonHeaders.WriteByte(':')
		canonHeaders.WriteString(v)
		canonHeaders.WriteByte('\n')
	}
	canon := strings.Join([]string{
		r.Method,
		canonicalURI(r.URL.Path),
		canonicalQueryString(r.URL.Query()),
		canonHeaders.String(),
		strings.Join(headers, ";"),
		payloadHash,
	}, "\n")
	return canon, nil
}

func canonicalHeaderValue(r *http.Request, name string) string {
	if name == "host" {
		return strings.TrimSpace(strings.ToLower(r.Host))
	}
	vals := r.Header.Values(name)
	if len(vals) == 0 {
		return ""
	}
	for i := range vals {
		vals[i] = strings.Join(strings.Fields(vals[i]), " ")
	}
	return strings.TrimSpace(strings.Join(vals, ","))
}

func canonicalURI(path string) string {
	if path == "" {
		return "/"
	}
	parts := strings.Split(path, "/")
	for i := range parts {
		parts[i] = awsEscape(parts[i])
	}
	uri := strings.Join(parts, "/")
	if !strings.HasPrefix(uri, "/") {
		uri = "/" + uri
	}
	return uri
}

func canonicalQueryString(values url.Values) string {
	if len(values) == 0 {
		return ""
	}
	keys := make([]string, 0, len(values))
	for k := range values {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	pairs := make([]string, 0)
	for _, k := range keys {
		vals := append([]string(nil), values[k]...)
		sort.Strings(vals)
		ek := awsEscape(k)
		for _, v := range vals {
			pairs = append(pairs, ek+"="+awsEscape(v))
		}
	}
	return strings.Join(pairs, "&")
}

func awsEscape(s string) string {
	e := url.QueryEscape(s)
	e = strings.ReplaceAll(e, "+", "%20")
	e = strings.ReplaceAll(e, "*", "%2A")
	e = strings.ReplaceAll(e, "%7E", "~")
	return e
}

func deriveSigningKey(secret, date, region, service, suffix string) []byte {
	log.Printf("DEBUG deriveSigningKey: secret=%s, date=%s, region=%s, service=%s, suffix=%s", secret, date, region, service, suffix)
	log.Printf("DEBUG secret bytes: %x", []byte(secret))
	log.Printf("DEBUG date bytes: %x", []byte(date))
	// Volc SDK uses no prefix, AWS4 uses "AWS4" prefix
	var kDate []byte
	if suffix == "request" {
		kDate = hmacSHA256([]byte(secret), date)
		log.Printf("DEBUG HMAC(secret, date) = %x", kDate)
	} else {
		kDate = hmacSHA256([]byte("AWS4"+secret), date)
	}
	log.Printf("DEBUG kDate: %x", kDate)
	kRegion := hmacSHA256(kDate, region)
	log.Printf("DEBUG kRegion: %x", kRegion)
	kService := hmacSHA256(kRegion, service)
	log.Printf("DEBUG kService: %x", kService)
	result := hmacSHA256(kService, suffix)
	log.Printf("DEBUG result: %x", result)
	return result
}

func hmacSHA256(key []byte, message string) []byte {
	h := hmac.New(sha256.New, key)
	_, _ = h.Write([]byte(message))
	return h.Sum(nil)
}

func sha256Hex(v []byte) string {
	s := sha256.Sum256(v)
	return hex.EncodeToString(s[:])
}

func constantTimeHexEqual(got, expected string) bool {
	if len(got) != len(expected) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(got), []byte(expected)) == 1
}

func containsHeader(signedHeaders []string, header string) bool {
	for _, h := range signedHeaders {
		if h == header {
			return true
		}
	}
	return false
}

func writeUnauthorized(w http.ResponseWriter, err error) {
	code := internalerrors.GetCode(err)
	if code == "" {
		code = internalerrors.ErrAuthFailed
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	if err := json.NewEncoder(w).Encode(map[string]any{
		"error": map[string]any{
			"code":    code,
			"message": err.Error(),
		},
	}); err != nil {
		return
	}
}
