package relay

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	internalerrors "github.com/jimeng-relay/server/internal/errors"
	"github.com/jimeng-relay/server/internal/logging"
	"github.com/jimeng-relay/server/internal/relay/upstream"
)

// maxDownstreamBodyBytes is the maximum request body size we accept from clients
// before relaying to upstream.
//
// Rationale: video i2v "first-tail" style requests may embed two local images.
// The client caps each inline image at ~5MiB, but JSON/form overhead makes the
// total payload slightly larger than 10MiB. Keep a clear safety margin.
const maxDownstreamBodyBytes int64 = 20 << 20

func logResponse(ctx context.Context, logger *slog.Logger, start time.Time, upstreamStatus int, err error) {
	latency := time.Since(start).Milliseconds()

	ctx = context.WithValue(ctx, logging.LatencyKey, latency)
	if upstreamStatus > 0 {
		ctx = context.WithValue(ctx, logging.UpstreamStatusKey, upstreamStatus)
	}
	if err != nil {
		ctx = context.WithValue(ctx, logging.ErrorClassKey, string(internalerrors.GetCode(err)))
	}

	if err != nil {
		logger.ErrorContext(ctx, "request finished", "error", err.Error())
	} else {
		logger.InfoContext(ctx, "request finished")
	}
}

func requestIDFromRequest(r *http.Request) string {
	if r != nil {
		if v := strings.TrimSpace(r.Header.Get("X-Request-Id")); v != "" {
			return v
		}
	}
	return "req_" + randomHex(8)
}

func randomHex(bytes int) string {
	if bytes <= 0 {
		bytes = 8
	}
	b := make([]byte, bytes)
	if _, err := rand.Read(b); err != nil {
		return hex.EncodeToString([]byte(time.Now().UTC().Format(time.RFC3339Nano)))
	}
	return hex.EncodeToString(b)
}

func headerToMapAny(h http.Header) map[string]any {
	if h == nil {
		return nil
	}
	out := make(map[string]any, len(h))
	for k, vals := range h {
		if len(vals) == 1 {
			out[k] = vals[0]
			continue
		}
		copyVals := append([]string(nil), vals...)
		out[k] = copyVals
	}
	return out
}

func decodeJSONMap(body []byte) map[string]any {
	if len(body) == 0 {
		return nil
	}
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	var m map[string]any
	if err := dec.Decode(&m); err != nil {
		return nil
	}
	return m
}

func pickForwardHeaders(src http.Header) http.Header {
	dst := make(http.Header)
	if contentType := strings.TrimSpace(src.Get("Content-Type")); contentType != "" {
		dst.Set("Content-Type", contentType)
	}
	if accept := strings.TrimSpace(src.Get("Accept")); accept != "" {
		dst.Set("Accept", accept)
	}
	return dst
}

func writeRelayPassthrough(w http.ResponseWriter, resp *upstream.Response) {
	if resp == nil {
		return
	}
	copyResponseHeaders(w.Header(), resp.Header)
	w.WriteHeader(resp.StatusCode)
	if _, err := w.Write(resp.Body); err != nil {
		return
	}
}

func copyResponseHeaders(dst, src http.Header) {
	if dst == nil || src == nil {
		return
	}
	for k, vals := range src {
		if len(vals) == 0 {
			continue
		}
		dst[k] = append([]string(nil), vals...)
	}
}

func writeRelayError(w http.ResponseWriter, err error, status int) {
	code := internalerrors.GetCode(err)
	if code == "" {
		code = internalerrors.ErrUnknown
	}
	w.Header().Set("Content-Type", "application/json")
	if status <= 0 {
		status = ErrorToStatus(err)
	}
	w.WriteHeader(status)
	if encErr := json.NewEncoder(w).Encode(map[string]any{
		"error": map[string]any{
			"code":    code,
			"message": err.Error(),
		},
	}); encErr != nil {
		return
	}
}

func hashRequestBody(body []byte) string {
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}

func readRequestBodyLimited(r *http.Request) ([]byte, error) {
	if r == nil || r.Body == nil {
		return nil, nil
	}
	limited := io.LimitReader(r.Body, maxDownstreamBodyBytes+1)
	body, err := io.ReadAll(limited)
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > maxDownstreamBodyBytes {
		return nil, errors.New("request body too large")
	}
	return body, nil
}

func ErrorToStatus(err error) int {
	code := internalerrors.GetCode(err)
	switch code {
	case internalerrors.ErrAuthFailed, internalerrors.ErrKeyRevoked, internalerrors.ErrKeyExpired, internalerrors.ErrInvalidSignature:
		return http.StatusUnauthorized
	case internalerrors.ErrRateLimited:
		return http.StatusTooManyRequests
	case internalerrors.ErrValidationFailed:
		return http.StatusBadRequest
	case internalerrors.ErrUpstreamFailed:
		return http.StatusBadGateway
	case internalerrors.ErrInternalError, internalerrors.ErrDatabaseError, internalerrors.ErrAuditFailed:
		return http.StatusInternalServerError
	default:
		return http.StatusInternalServerError
	}
}
