package upstream

import (
	"context"
	"testing"
)

func TestAPIKeyIDContext(t *testing.T) {
	ctx := context.Background()

	// Test empty context
	if id := GetAPIKeyID(ctx); id != "" {
		t.Errorf("expected empty string, got %q", id)
	}

	// Test setting and getting
	expected := "test-api-key-id"
	ctx = WithAPIKeyID(ctx, expected)
	if id := GetAPIKeyID(ctx); id != expected {
		t.Errorf("expected %q, got %q", expected, id)
	}

	// Test with different context
	ctx2 := context.Background()
	if id := GetAPIKeyID(ctx2); id != "" {
		t.Errorf("expected empty string for new context, got %q", id)
	}
}
