package upstream

import (
	"context"
	"testing"
	"time"

	"github.com/jimeng-relay/server/internal/config"
)

func TestClient_ReassignCancelledWaiterSlot_NoWaiters_DoesNotOccupySemaphore(t *testing.T) {
	c, err := NewClient(config.Config{
		Credentials: config.Credentials{AccessKey: "ak", SecretKey: "sk"},
		Region:      "cn-north-1",
		Host:        "example.com",
		Timeout:     500 * time.Millisecond,
	}, Options{
		MaxConcurrent: 1,
		MaxQueue:      10,
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	if removed := c.removeWaiter(&queueWaiter{ready: make(chan struct{})}); removed {
		t.Fatalf("expected waiter to be absent")
	}

	c.reassignCancelledWaiterSlot()

	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()

	if err := c.acquire(ctx); err != nil {
		t.Fatalf("follow-up acquire should not deadlock after reassignment cleanup: %v", err)
	}
	c.release()
}
