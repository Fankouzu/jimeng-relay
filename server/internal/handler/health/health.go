package health

import (
	"encoding/json"
	"net/http"
)

// Handler provides health check endpoints
type Handler struct {
	dbReady func() bool
}

// NewHandler creates a new health check handler
func NewHandler(dbReady func() bool) *Handler {
	return &Handler{dbReady: dbReady}
}

// Health returns liveness status (process is running)
func (h *Handler) Health(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// Ready returns readiness status (can handle requests)
func (h *Handler) Ready(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if h.dbReady == nil || h.dbReady() {
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
		return
	}

	w.WriteHeader(http.StatusServiceUnavailable)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"status":  "error",
		"message": "database not ready",
	})
}

// Routes returns a mux with health check endpoints registered
func (h *Handler) Routes() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", h.Health)
	mux.HandleFunc("/ready", h.Ready)
	return mux
}
