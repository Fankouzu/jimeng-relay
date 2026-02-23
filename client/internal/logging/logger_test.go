package logging

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"testing"
)

func TestLogRedaction(t *testing.T) {
	var buf bytes.Buffer
	opts := &slog.HandlerOptions{Level: slog.LevelInfo}
	jsonHandler := slog.NewJSONHandler(&buf, opts)
	logger := slog.New(&RedactingHandler{Handler: jsonHandler})

	ctx := context.WithValue(context.Background(), RequestIDKey, "test-req-123")

	logger.InfoContext(ctx, "test message",
		"ak", "AKID12345678",
		"sk", "SECRET12345678",
		"other", "public info",
		slog.Group("credentials",
			"access_key", "AKID87654321",
			"secret_key", "SECRET87654321",
		),
	)

	var result map[string]any
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal log output: %v", err)
	}

	if result["request_id"] != "test-req-123" {
		t.Errorf("expected request_id test-req-123, got %v", result["request_id"])
	}

	if result["ak"] != "AKID..." {
		t.Errorf("expected ak AKID..., got %v", result["ak"])
	}

	if result["sk"] != "***" {
		t.Errorf("expected sk ***, got %v", result["sk"])
	}

	if result["other"] != "public info" {
		t.Errorf("expected other public info, got %v", result["other"])
	}

	creds, ok := result["credentials"].(map[string]any)
	if !ok {
		t.Fatalf("credentials group not found or not a map")
	}
	if creds["access_key"] != "AKID..." {
		t.Errorf("expected nested access_key AKID..., got %v", creds["access_key"])
	}
	if creds["secret_key"] != "***" {
		t.Errorf("expected nested secret_key ***, got %v", creds["secret_key"])
	}
}
