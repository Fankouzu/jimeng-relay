package keymanager

import (
	"context"
	"testing"

	"github.com/jimeng-relay/server/internal/errors"
)

func TestKeyManager_AcquireKey(t *testing.T) {
	svc := NewService(nil)
	ctx := context.Background()

	t.Run("HappyPath", func(t *testing.T) {
		handle, err := svc.AcquireKey(ctx, "key1", "req1")
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if handle == nil {
			t.Fatal("expected handle, got nil")
		}
		if handle.apiKeyID != "key1" {
			t.Errorf("expected apiKeyID key1, got %s", handle.apiKeyID)
		}
	})

	t.Run("EmptyKey", func(t *testing.T) {
		_, err := svc.AcquireKey(ctx, "", "req2")
		if err == nil {
			t.Fatal("expected error for empty key, got nil")
		}
		if errors.GetCode(err) != errors.ErrAuthFailed {
			t.Errorf("expected ErrAuthFailed, got %v", err)
		}
	})

	t.Run("RateLimited", func(t *testing.T) {
		_, err := svc.AcquireKey(ctx, "key2", "req3")
		if err != nil {
			t.Fatalf("first acquire failed: %v", err)
		}

		_, err = svc.AcquireKey(ctx, "key2", "req4")
		if err == nil {
			t.Fatal("expected error for second acquire, got nil")
		}
		if errors.GetCode(err) != errors.ErrRateLimited {
			t.Errorf("expected ErrRateLimited, got %v", err)
		}
	})
}

func TestKeyManager_Release(t *testing.T) {
	svc := NewService(nil)
	ctx := context.Background()

	t.Run("HappyPath", func(t *testing.T) {
		handle, err := svc.AcquireKey(ctx, "key1", "req1")
		if err != nil {
			t.Fatalf("acquire failed: %v", err)
		}

		handle.Release()

		handle2, err := svc.AcquireKey(ctx, "key1", "req2")
		if err != nil {
			t.Fatalf("acquire after release failed: %v", err)
		}
		if handle2 == nil {
			t.Fatal("expected handle after release, got nil")
		}
	})

	t.Run("Idempotent", func(t *testing.T) {
		handle, err := svc.AcquireKey(ctx, "key2", "req3")
		if err != nil {
			t.Fatalf("acquire failed: %v", err)
		}

		handle.Release()
		handle.Release()

		_, err = svc.AcquireKey(ctx, "key2", "req4")
		if err != nil {
			t.Fatalf("acquire after double release failed: %v", err)
		}
	})
}

func TestKeyManager_RevokeKey(t *testing.T) {
	svc := NewService(nil)
	ctx := context.Background()

	t.Run("RevokeBeforeAcquire", func(t *testing.T) {
		svc.RevokeKey("key1")
		_, err := svc.AcquireKey(ctx, "key1", "req1")
		if err == nil {
			t.Fatal("expected error for revoked key, got nil")
		}
		if errors.GetCode(err) != errors.ErrKeyRevoked {
			t.Errorf("expected ErrKeyRevoked, got %v", err)
		}
	})

	t.Run("RevokeWhileInUse", func(t *testing.T) {
		handle, err := svc.AcquireKey(ctx, "key2", "req2")
		if err != nil {
			t.Fatalf("acquire failed: %v", err)
		}

		svc.RevokeKey("key2")

		handle.Release()

		_, err = svc.AcquireKey(ctx, "key2", "req3")
		if err == nil {
			t.Fatal("expected error for revoked key after release, got nil")
		}
		if errors.GetCode(err) != errors.ErrKeyRevoked {
			t.Errorf("expected ErrKeyRevoked, got %v", err)
		}
	})
}

func TestKeyManager_CleanupKey(t *testing.T) {
	svc := NewService(nil)
	ctx := context.Background()

	t.Run("CleanupUnused", func(t *testing.T) {
		handle, _ := svc.AcquireKey(ctx, "key1", "req1")
		handle.Release()

		svc.CleanupKey("key1")

		handle2, err := svc.AcquireKey(ctx, "key1", "req2")
		if err != nil {
			t.Fatalf("acquire after cleanup failed: %v", err)
		}
		if handle2 == nil {
			t.Fatal("expected handle after cleanup, got nil")
		}
	})

	t.Run("CleanupInUse", func(t *testing.T) {
		_, _ = svc.AcquireKey(ctx, "key2", "req3")
		svc.CleanupKey("key2")

		_, err := svc.AcquireKey(ctx, "key2", "req4")
		if errors.GetCode(err) != errors.ErrRateLimited {
			t.Errorf("expected ErrRateLimited (not deleted), got %v", err)
		}
	})

	t.Run("CleanupRevoked", func(t *testing.T) {
		svc.RevokeKey("key3")
		svc.CleanupKey("key3")

		_, err := svc.AcquireKey(ctx, "key3", "req5")
		if errors.GetCode(err) != errors.ErrKeyRevoked {
			t.Errorf("expected ErrKeyRevoked (not deleted), got %v", err)
		}
	})
}
