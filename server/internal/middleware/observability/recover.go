package observability

import (
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"
)

type statusTrackingResponseWriter struct {
	http.ResponseWriter
	wroteHeader bool
}

func (w *statusTrackingResponseWriter) WriteHeader(statusCode int) {
	w.wroteHeader = true
	w.ResponseWriter.WriteHeader(statusCode)
}

func (w *statusTrackingResponseWriter) Write(p []byte) (int, error) {
	if !w.wroteHeader {
		w.wroteHeader = true
	}
	return w.ResponseWriter.Write(p)
}

func RecoverMiddleware(logger *slog.Logger) func(http.Handler) http.Handler {
	if logger == nil {
		logger = slog.Default()
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			tw := &statusTrackingResponseWriter{ResponseWriter: w}
			defer func() {
				if rec := recover(); rec != nil {
					logger.ErrorContext(
						r.Context(),
						"panic recovered",
						"panic", fmt.Sprint(rec),
						"path", r.URL.Path,
						"method", r.Method,
						"stack", string(debug.Stack()),
					)
					if !tw.wroteHeader {
						tw.Header().Set("Content-Type", "application/json")
						tw.WriteHeader(http.StatusInternalServerError)
						if _, err := tw.Write([]byte(`{"error":{"code":"INTERNAL_ERROR","message":"internal server error"}}`)); err != nil {
						}
					}
				}
			}()

			next.ServeHTTP(tw, r)
		})
	}
}
