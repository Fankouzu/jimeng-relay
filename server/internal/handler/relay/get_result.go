package relay

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	internalerrors "github.com/jimeng-relay/server/internal/errors"
	"github.com/jimeng-relay/server/internal/logging"
	"github.com/jimeng-relay/server/internal/middleware/sigv4"
	"github.com/jimeng-relay/server/internal/models"
	"github.com/jimeng-relay/server/internal/relay/upstream"
	auditservice "github.com/jimeng-relay/server/internal/service/audit"
)

const getResultAction = "CVSync2AsyncGetResult"

type getResultClient interface {
	GetResult(ctx context.Context, body []byte, headers http.Header) (*upstream.Response, error)
}

type GetResultHandler struct {
	client getResultClient
	audit  *auditservice.Service
	logger *slog.Logger
}

func NewGetResultHandler(client getResultClient, auditSvc *auditservice.Service, logger *slog.Logger) *GetResultHandler {
	if logger == nil {
		logger = slog.Default()
	}
	return &GetResultHandler{client: client, audit: auditSvc, logger: logger}
}

func (h *GetResultHandler) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/get-result", h.handleGetResult)
	mux.HandleFunc("/", h.handleCompatibleGetResult)
	return mux
}

func (h *GetResultHandler) handleCompatibleGetResult(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" || r.Method != http.MethodPost || r.URL.Query().Get("Action") != getResultAction {
		http.NotFound(w, r)
		return
	}
	h.proxyGetResult(w, r)
}

func (h *GetResultHandler) handleGetResult(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeRelayError(w, internalerrors.New(internalerrors.ErrValidationFailed, "method not allowed", nil), http.StatusMethodNotAllowed)
		return
	}
	h.proxyGetResult(w, r)
}

func (h *GetResultHandler) proxyGetResult(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	var upstreamStatus int
	var finalErr error

	reqID := requestIDFromRequest(r)
	ctx := context.WithValue(r.Context(), logging.RequestIDKey, reqID)

	defer func() {
		logResponse(ctx, h.logger, start, upstreamStatus, finalErr)
	}()

	if h.client == nil {
		finalErr = internalerrors.New(internalerrors.ErrInternalError, "get-result upstream client is not configured", nil)
		writeRelayError(w, finalErr, http.StatusInternalServerError)
		return
	}
	if h.audit == nil {
		finalErr = internalerrors.New(internalerrors.ErrInternalError, "audit service is not configured", nil)
		writeRelayError(w, finalErr, http.StatusInternalServerError)
		return
	}

	apiKeyID, ok := r.Context().Value(sigv4.ContextAPIKeyID).(string)
	if !ok {
		finalErr = internalerrors.New(internalerrors.ErrInternalError, "invalid api_key_id type in context", nil)
		writeRelayError(w, finalErr, http.StatusInternalServerError)
		return
	}
	apiKeyID = strings.TrimSpace(apiKeyID)
	if apiKeyID == "" {
		finalErr = internalerrors.New(internalerrors.ErrInternalError, "missing api_key_id in context", nil)
		writeRelayError(w, finalErr, http.StatusInternalServerError)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		finalErr = internalerrors.New(internalerrors.ErrInternalError, "read downstream request body", err)
		writeRelayError(w, finalErr, http.StatusInternalServerError)
		return
	}

	headers := pickForwardHeaders(r.Header)
	call := auditservice.RelayCall{
		RequestID:         reqID,
		APIKeyID:          apiKeyID,
		Action:            models.DownstreamActionCVSync2AsyncGetResult,
		Method:            r.Method,
		Path:              r.URL.Path,
		Query:             r.URL.RawQuery,
		ClientIP:          strings.TrimSpace(r.RemoteAddr),
		DownstreamHeaders: headerToMapAny(r.Header),
		DownstreamBody:    decodeJSONMap(body),
		Upstream: auditservice.UpstreamAttempt{
			AttemptNumber:  1,
			UpstreamAction: getResultAction,
			RequestHeaders: headerToMapAny(headers),
			RequestBody:    nil,
		},
	}
	if err := h.audit.RecordRelayDownstream(ctx, call); err != nil {
		finalErr = err
		writeRelayError(w, finalErr, http.StatusInternalServerError)
		return
	}

	resp, callErr := h.client.GetResult(ctx, body, headers)
	if resp != nil {
		upstreamStatus = resp.StatusCode
		latencyMs := time.Since(start).Milliseconds()
		var upstreamErr *string
		if callErr != nil {
			s := callErr.Error()
			upstreamErr = &s
			h.logger.WarnContext(ctx, "get-result upstream returned error status", "error", callErr.Error(), "status", resp.StatusCode)
		}
		call.Upstream.ResponseStatus = resp.StatusCode
		call.Upstream.ResponseHeaders = headerToMapAny(resp.Header)
		call.Upstream.ResponseBody = nil
		call.Upstream.LatencyMs = latencyMs
		call.Upstream.Error = upstreamErr
		if err := h.audit.RecordRelayUpstreamAndEvents(ctx, call); err != nil {
			finalErr = err
			writeRelayError(w, finalErr, http.StatusInternalServerError)
			return
		}
		writeRelayPassthrough(w, resp)
		return
	}
	if callErr != nil {
		finalErr = internalerrors.New(internalerrors.ErrUpstreamFailed, "get-result upstream request failed", callErr)
		writeRelayError(w, finalErr, http.StatusBadGateway)
		return
	}

	finalErr = internalerrors.New(internalerrors.ErrUpstreamFailed, "get-result upstream returned empty response", nil)
	writeRelayError(w, finalErr, http.StatusBadGateway)
}
