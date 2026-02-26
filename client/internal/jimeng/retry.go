package jimeng

import (
	"context"
	"errors"
	"math"
	"math/rand"
	"net"
	"syscall"
	"time"

	internalerrors "github.com/jimeng-relay/client/internal/errors"
)

type RetryConfig struct {
	MaxRetries   int
	InitialDelay time.Duration
	MaxDelay     time.Duration
	Multiplier   float64
}

var DefaultRetryConfig = RetryConfig{
	MaxRetries:   3,
	InitialDelay: 500 * time.Millisecond,
	MaxDelay:     30 * time.Second,
	Multiplier:   2,
}

type RetryableFunc func() error

func DoWithRetry(ctx context.Context, cfg RetryConfig, fn RetryableFunc) error {
	if fn == nil {
		return internalerrors.New(internalerrors.ErrValidationFailed, "retry function is nil", nil)
	}

	cfg = normalizeRetryConfig(cfg)
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))

	var lastErr error
	for attempt := 0; attempt <= cfg.MaxRetries; attempt++ {
		if err := ctx.Err(); err != nil {
			return internalerrors.New(internalerrors.ErrTimeout, "context done before attempt", err)
		}

		err := fn()
		if err == nil {
			return nil
		}
		lastErr = err

		if attempt >= cfg.MaxRetries || !IsRetryable(err) {
			return err
		}

		delay := backoffDelay(rng, cfg, attempt)
		if err := sleepWithContext(ctx, delay); err != nil {
			return internalerrors.New(internalerrors.ErrTimeout, "context done during retry backoff", err)
		}
	}

	return lastErr
}

func IsRetryable(err error) bool {
	if err == nil {
		return false
	}

	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}

	var ie *internalerrors.Error
	if errors.As(err, &ie) {
		if ie.Code == internalerrors.ErrRateLimited {
			return true
		}
	}

	if status, ok := statusCodeFromError(err); ok {
		if status == 429 {
			return true
		}
		if status >= 500 && status <= 599 {
			return true
		}
	}

	if isNetworkError(err) {
		return true
	}

	return false
}

func normalizeRetryConfig(cfg RetryConfig) RetryConfig {
	if cfg.MaxRetries < 0 {
		cfg.MaxRetries = 0
	}
	if cfg.InitialDelay <= 0 {
		cfg.InitialDelay = DefaultRetryConfig.InitialDelay
	}
	if cfg.MaxDelay <= 0 {
		cfg.MaxDelay = DefaultRetryConfig.MaxDelay
	}
	if cfg.Multiplier <= 1 {
		cfg.Multiplier = DefaultRetryConfig.Multiplier
	}
	if cfg.MaxDelay < cfg.InitialDelay {
		cfg.MaxDelay = cfg.InitialDelay
	}
	return cfg
}

func backoffDelay(rng *rand.Rand, cfg RetryConfig, attemptIndex int) time.Duration {
	pow := math.Pow(cfg.Multiplier, float64(attemptIndex))
	d := time.Duration(float64(cfg.InitialDelay) * pow)
	if d > cfg.MaxDelay {
		d = cfg.MaxDelay
	}
	if d <= 0 {
		return 0
	}

	const minJitter = 0.5
	const maxJitter = 1.5
	jitterFactor := minJitter + rng.Float64()*(maxJitter-minJitter)
	jd := time.Duration(float64(d) * jitterFactor)
	if jd > cfg.MaxDelay {
		jd = cfg.MaxDelay
	}
	if jd < 0 {
		return 0
	}
	return jd
}

func sleepWithContext(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return ctx.Err()
	}

	t := time.NewTimer(d)
	defer t.Stop()

	select {
	case <-t.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func isNetworkError(err error) bool {
	var ne net.Error
	if errors.As(err, &ne) {
		return true
	}

	if errors.Is(err, syscall.ECONNRESET) ||
		errors.Is(err, syscall.ECONNREFUSED) ||
		errors.Is(err, syscall.ETIMEDOUT) ||
		errors.Is(err, syscall.EPIPE) {
		return true
	}

	return false
}

func statusCodeFromError(err error) (int, bool) {
	type statusCoder interface{ StatusCode() int }
	type httpStatusCoder interface{ HTTPStatusCode() int }
	type getStatusCoder interface{ GetStatusCode() int }

	var sc statusCoder
	if errors.As(err, &sc) {
		return sc.StatusCode(), true
	}
	var hsc httpStatusCoder
	if errors.As(err, &hsc) {
		return hsc.HTTPStatusCode(), true
	}
	var gsc getStatusCoder
	if errors.As(err, &gsc) {
		return gsc.GetStatusCode(), true
	}

	return 0, false
}
