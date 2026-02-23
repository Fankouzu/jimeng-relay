package logging

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
)

func TestRedactingHandler(t *testing.T) {
	var buf bytes.Buffer
	handler := &RedactingHandler{
		Handler: slog.NewJSONHandler(&buf, nil),
	}
	logger := slog.New(handler)

	ctx := context.WithValue(context.Background(), RequestIDKey, "test-req-id")

	logger.InfoContext(ctx, "test message",
		"authorization", "Bearer some-very-long-token-that-should-be-redacted",
		"X-Date", "20231027T100000Z",
		"X-Amz-Date", "20231027T100000Z",
		"secret_key", "super-secret",
		"signature", "some-signature",
		"binary_data_base64", strings.Repeat("a", 100),
		"nested", slog.GroupValue(
			slog.String("sk", "nested-secret"),
			slog.String("public", "visible"),
		),
	)

	var result map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal log: %v", err)
	}

	if result["request_id"] != "test-req-id" {
		t.Errorf("expected request_id test-req-id, got %v", result["request_id"])
	}

	auth, ok := result["authorization"].(string)
	if !ok {
		t.Fatalf("authorization should be string, got %T", result["authorization"])
	}
	if !strings.HasSuffix(auth, "...") || len(auth) != 23 {
		t.Errorf("authorization not correctly redacted: %s", auth)
	}

	if result["X-Date"] != "***" {
		t.Errorf("X-Date not redacted: %v", result["X-Date"])
	}
	if result["X-Amz-Date"] != "***" {
		t.Errorf("X-Amz-Date not redacted: %v", result["X-Amz-Date"])
	}

	if result["secret_key"] != "***" {
		t.Errorf("secret_key not redacted: %v", result["secret_key"])
	}

	if result["signature"] != "***" {
		t.Errorf("signature not redacted: %v", result["signature"])
	}

	bin, ok := result["binary_data_base64"].(string)
	if !ok {
		t.Fatalf("binary_data_base64 should be string, got %T", result["binary_data_base64"])
	}
	if !strings.HasSuffix(bin, "...") || len(bin) != 53 {
		t.Errorf("binary_data_base64 not correctly truncated: %s", bin)
	}

	nested, ok := result["nested"].(map[string]interface{})
	if !ok {
		t.Fatalf("nested should be map, got %T", result["nested"])
	}
	if nested["sk"] != "***" {
		t.Errorf("nested sk not redacted: %v", nested["sk"])
	}
	if nested["public"] != "visible" {
		t.Errorf("nested public field modified: %v", nested["public"])
	}
}
