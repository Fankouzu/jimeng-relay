package logging

import (
	"context"
	"log/slog"
	"os"
	"strings"
)

type contextKey string

const RequestIDKey contextKey = "request_id"

type RedactingHandler struct {
	slog.Handler
}

func NewLogger(level slog.Level) *slog.Logger {
	opts := &slog.HandlerOptions{
		Level: level,
	}
	handler := slog.NewJSONHandler(os.Stdout, opts)
	return slog.New(&RedactingHandler{Handler: handler})
}

func (h *RedactingHandler) Handle(ctx context.Context, r slog.Record) error {
	if reqID, ok := ctx.Value(RequestIDKey).(string); ok {
		r.AddAttrs(slog.String("request_id", reqID))
	}

	newRecord := slog.NewRecord(r.Time, r.Level, r.Message, r.PC)
	r.Attrs(func(a slog.Attr) bool {
		newRecord.AddAttrs(h.redactAttr(a))
		return true
	})

	return h.Handler.Handle(ctx, newRecord)
}

func (h *RedactingHandler) redactAttr(a slog.Attr) slog.Attr {
	key := strings.ToLower(a.Key)

	switch {
	case key == "authorization":
		val := a.Value.String()
		if len(val) > 20 {
			return slog.String(a.Key, val[:20]+"...")
		}
		return slog.String(a.Key, val+"...")

	case key == "x-date" || key == "x-security-token" || strings.HasPrefix(key, "x-amz-"):
		return slog.String(a.Key, "***")

	case key == "secret_key" || key == "sk" || key == "secretkey" || key == "access_key_secret":
		return slog.String(a.Key, "***")

	case key == "signature" || key == "x-amz-signature":
		return slog.String(a.Key, "***")

	case key == "binary_data_base64":
		val := a.Value.String()
		if len(val) > 50 {
			return slog.String(a.Key, val[:50]+"...")
		}
		return slog.String(a.Key, val)
	}

	if a.Value.Kind() == slog.KindGroup {
		attrs := a.Value.Group()
		newAttrs := make([]slog.Attr, len(attrs))
		for i, attr := range attrs {
			newAttrs[i] = h.redactAttr(attr)
		}
		return slog.Group(a.Key, anySliceToAny(newAttrs)...)
	}

	return a
}

func anySliceToAny(attrs []slog.Attr) []any {
	args := make([]any, len(attrs))
	for i, a := range attrs {
		args[i] = a
	}
	return args
}
