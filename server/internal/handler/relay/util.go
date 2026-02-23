package relay

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"

	internalerrors "github.com/jimeng-relay/server/internal/errors"
	"github.com/jimeng-relay/server/internal/logging"
	"github.com/jimeng-relay/server/internal/relay/upstream"
)

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
	if contentType := strings.TrimSpace(resp.Header.Get("Content-Type")); contentType != "" {
		w.Header().Set("Content-Type", contentType)
	}
	w.WriteHeader(resp.StatusCode)
	if _, err := w.Write(resp.Body); err != nil {
		return
	}
}

func writeRelayError(w http.ResponseWriter, err error, status int) {
	code := internalerrors.GetCode(err)
	if code == "" {
		code = internalerrors.ErrUnknown
	}
	w.Header().Set("Content-Type", "application/json")
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
