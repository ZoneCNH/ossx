package ossx

import (
	"context"
	"errors"
	"math"
	"sync"
	"time"
)

// retryPolicy bounds retry behavior for adapter operations (FR-003 / FR-005).
// Derived from Config.Retry at construction. Kept local to keep go.mod
// provider-SDK-only (mirrors sibling adapters); semantically aligned with
// resiliencx retry.Policy so a future swap is mechanical.
type retryPolicy struct {
	MaxAttempts int
	InitialWait time.Duration
	MaxWait     time.Duration
	Multiplier  float64
}

func retryPolicyFromConfig(c RetryConfig) retryPolicy {
	p := retryPolicy{
		MaxAttempts: c.MaxAttempts,
		InitialWait: c.InitialWait,
		MaxWait:     c.MaxWait,
		Multiplier:  c.Multiplier,
	}
	if p.MaxAttempts <= 0 {
		p.MaxAttempts = 3
	}
	if p.InitialWait <= 0 {
		p.InitialWait = 100 * time.Millisecond
	}
	if p.MaxWait <= 0 {
		p.MaxWait = 5 * time.Second
	}
	if p.Multiplier <= 0 {
		p.Multiplier = 2
	}
	return p
}

// delay computes the backoff for the given (1-based) attempt.
func (p retryPolicy) delay(attempt int) time.Duration {
	if attempt <= 1 {
		return 0
	}
	d := float64(p.InitialWait) * math.Pow(p.Multiplier, float64(attempt-2))
	if d > float64(p.MaxWait) {
		d = float64(p.MaxWait)
	}
	return time.Duration(d)
}

// withRetry runs fn up to MaxAttempts, retrying when classifyError reports
// retryClassRetryable. Context cancellation is honored between attempts.
// Returns the last error if all attempts fail. Non-retryable / fatal errors
// return immediately.
func (p retryPolicy) withRetry(ctx context.Context, op string, fn func(context.Context) error) error {
	var lastErr error
	for attempt := 1; attempt <= p.MaxAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return wrapError(ErrorKindCanceled, op, "retry cancelled", err)
		}
		err := fn(ctx)
		if err == nil {
			return nil
		}
		lastErr = err
		switch classifyError(err) {
		case retryClassFatal:
			return err
		case retryClassNonRetryable:
			return err
		case retryClassRetryable:
			// fall through to backoff + next attempt
		}
		if attempt == p.MaxAttempts {
			break
		}
		wait := p.delay(attempt + 1)
		select {
		case <-ctx.Done():
			return wrapError(ErrorKindCanceled, op, "retry backoff cancelled", ctx.Err())
		case <-time.After(wait):
		}
	}
	return lastErr
}

// circuitBreaker is a minimal concurrency-safe breaker. Opens after threshold
// consecutive failures; half-open after cooldown. Kept local to avoid a
// resiliencx import (mirrors sibling adapter convention).
type circuitBreaker struct {
	threshold int
	cooldown  time.Duration

	mu          sync.Mutex
	failures    int
	state       breakerState
	openedAt    time.Time
}

type breakerState int

const (
	breakerClosed breakerState = iota
	breakerOpen
	breakerHalfOpen
)

func newCircuitBreaker(threshold int, cooldown time.Duration) *circuitBreaker {
	if threshold <= 0 {
		threshold = 5
	}
	if cooldown <= 0 {
		cooldown = 30 * time.Second
	}
	return &circuitBreaker{threshold: threshold, cooldown: cooldown, state: breakerClosed}
}

func (cb *circuitBreaker) allow() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	switch cb.state {
	case breakerClosed:
		return true
	case breakerOpen:
		if time.Since(cb.openedAt) >= cb.cooldown {
			cb.state = breakerHalfOpen
			return true
		}
		return false
	case breakerHalfOpen:
		return true
	default:
		return true
	}
}

func (cb *circuitBreaker) recordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.failures = 0
	cb.state = breakerClosed
}

func (cb *circuitBreaker) recordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.failures++
	if cb.failures >= cb.threshold {
		cb.state = breakerOpen
		cb.openedAt = time.Now()
	}
}

// do runs fn through the breaker + retry policy. Used by blobstore operations.
// Non-retryable errors (config/validation/not-found) short-circuit the retry.
func (cb *circuitBreaker) do(ctx context.Context, op string, p retryPolicy, fn func(context.Context) error) error {
	if !cb.allow() {
		return newError(ErrorKindUnavailable, op, "circuit breaker open")
	}
	err := p.withRetry(ctx, op, fn)
	if err == nil {
		cb.recordSuccess()
		return nil
	}
	if isRetryable(err) {
		cb.recordFailure()
	}
	return err
}

// errCircuitOpen distinguishes breaker-open from other unavailable errors.
var errCircuitOpen = newError(ErrorKindUnavailable, "", "circuit breaker open")

// isErrCircuitOpen reports whether err is the breaker-open sentinel.
func isErrCircuitOpen(err error) bool {
	var e *Error
	if errors.As(err, &e) {
		return e.Kind == ErrorKindUnavailable && e.Message == "circuit breaker open"
	}
	return false
}
