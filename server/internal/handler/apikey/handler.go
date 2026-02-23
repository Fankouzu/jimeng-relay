package apikey

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	internalerrors "github.com/jimeng-relay/server/internal/errors"
	service "github.com/jimeng-relay/server/internal/service/apikey"
)

type apiKeyService interface {
	Create(ctx context.Context, req service.CreateRequest) (service.KeyWithSecret, error)
	List(ctx context.Context) ([]service.KeyView, error)
	Revoke(ctx context.Context, id string) error
	Rotate(ctx context.Context, req service.RotateRequest) (service.KeyWithSecret, error)
}

type Handler struct {
	service apiKeyService
	logger  *slog.Logger
}

func NewHandler(svc apiKeyService, logger *slog.Logger) *Handler {
	if logger == nil {
		logger = slog.Default()
	}
	return &Handler{service: svc, logger: logger}
}

func (h *Handler) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/keys", h.handleKeys)
	mux.HandleFunc("/v1/keys/", h.handleKeyAction)
	return mux
}

func (h *Handler) handleKeys(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		h.createKey(w, r)
	case http.MethodGet:
		h.listKeys(w, r)
	default:
		writeError(w, internalerrors.New(internalerrors.ErrValidationFailed, "method not allowed", nil), http.StatusMethodNotAllowed)
	}
}

type createRequest struct {
	Description string `json:"description"`
	ExpiresAt   string `json:"expires_at"`
}

func (h *Handler) createKey(w http.ResponseWriter, r *http.Request) {
	var req createRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, internalerrors.New(internalerrors.ErrValidationFailed, "invalid request body", err), http.StatusBadRequest)
		return
	}

	var expiresAt *time.Time
	if strings.TrimSpace(req.ExpiresAt) != "" {
		parsed, err := time.Parse(time.RFC3339, req.ExpiresAt)
		if err != nil {
			writeError(w, internalerrors.New(internalerrors.ErrValidationFailed, "expires_at must be RFC3339", err), http.StatusBadRequest)
			return
		}
		expiresAt = &parsed
	}

	created, err := h.service.Create(r.Context(), service.CreateRequest{
		Description: req.Description,
		ExpiresAt:   expiresAt,
	})
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

func (h *Handler) listKeys(w http.ResponseWriter, r *http.Request) {
	keys, err := h.service.List(r.Context())
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": keys})
}

func (h *Handler) handleKeyAction(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/v1/keys/")
	parts := strings.Split(path, "/")
	if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" {
		writeError(w, internalerrors.New(internalerrors.ErrValidationFailed, "invalid path", nil), http.StatusNotFound)
		return
	}
	id := strings.TrimSpace(parts[0])
	action := parts[1]

	switch action {
	case "revoke":
		if r.Method != http.MethodPost {
			writeError(w, internalerrors.New(internalerrors.ErrValidationFailed, "method not allowed", nil), http.StatusMethodNotAllowed)
			return
		}
		if err := h.service.Revoke(r.Context(), id); err != nil {
			writeServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"id": id, "status": "revoked"})
	case "rotate":
		if r.Method != http.MethodPost {
			writeError(w, internalerrors.New(internalerrors.ErrValidationFailed, "method not allowed", nil), http.StatusMethodNotAllowed)
			return
		}
		h.rotateKey(w, r, id)
	default:
		writeError(w, internalerrors.New(internalerrors.ErrValidationFailed, "unknown action", nil), http.StatusNotFound)
	}
}

type rotateRequest struct {
	Description        *string `json:"description"`
	ExpiresAt          string  `json:"expires_at"`
	GracePeriodSeconds int64   `json:"grace_period_seconds"`
}

func (h *Handler) rotateKey(w http.ResponseWriter, r *http.Request, id string) {
	var req rotateRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, internalerrors.New(internalerrors.ErrValidationFailed, "invalid request body", err), http.StatusBadRequest)
		return
	}

	var expiresAt *time.Time
	if strings.TrimSpace(req.ExpiresAt) != "" {
		parsed, err := time.Parse(time.RFC3339, req.ExpiresAt)
		if err != nil {
			writeError(w, internalerrors.New(internalerrors.ErrValidationFailed, "expires_at must be RFC3339", err), http.StatusBadRequest)
			return
		}
		expiresAt = &parsed
	}

	rotated, err := h.service.Rotate(r.Context(), service.RotateRequest{
		ID:          id,
		Description: req.Description,
		ExpiresAt:   expiresAt,
		GracePeriod: time.Duration(req.GracePeriodSeconds) * time.Second,
	})
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, rotated)
}

func decodeJSON(r *http.Request, dst any) error {
	if r.Body == nil {
		return nil
	}
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		if errors.Is(err, http.ErrBodyReadAfterClose) {
			return nil
		}
		return err
	}
	return nil
}

func writeServiceError(w http.ResponseWriter, err error) {
	code := internalerrors.GetCode(err)
	switch code {
	case internalerrors.ErrValidationFailed:
		writeError(w, err, http.StatusBadRequest)
	case internalerrors.ErrKeyExpired, internalerrors.ErrKeyRevoked:
		writeError(w, err, http.StatusForbidden)
	default:
		writeError(w, err, http.StatusInternalServerError)
	}
}

func writeError(w http.ResponseWriter, err error, status int) {
	code := internalerrors.GetCode(err)
	if code == "" {
		code = internalerrors.ErrUnknown
	}
	writeJSON(w, status, map[string]any{
		"error": map[string]any{
			"code":    code,
			"message": err.Error(),
		},
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		return
	}
}
