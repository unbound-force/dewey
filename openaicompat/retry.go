package openaicompat

import (
	"context"
	"math"
	"math/rand/v2"
	"strconv"
	"time"
)

const (
	// MaxRetries is the number of retries after the initial request
	// (FR-012: initial attempt plus at most five retries).
	MaxRetries = 5
	// BaseDelay is the initial exponential backoff delay (FR-011).
	BaseDelay = 1 * time.Second
	// MaxDelay caps both Retry-After and exponential backoff (FR-011).
	MaxDelay = 60 * time.Second
	// jitterFraction is the plus-or-minus jitter applied to the
	// exponential delay.
	jitterFraction = 0.25
)

// RetryDelay computes the delay before the next retry. A positive
// integer-seconds Retry-After value is honored and capped at MaxDelay.
// Anything else (missing, negative, zero, non-numeric, or HTTP-date) is
// treated as absent and exponential backoff applies: BaseDelay doubled
// per attempt with plus-or-minus 25 percent jitter, capped at MaxDelay.
//
// attempt is zero-based: attempt 0 is the delay before the first retry.
func RetryDelay(attempt int, retryAfter string) time.Duration {
	if delay, ok := parseRetryAfter(retryAfter); ok {
		return delay
	}

	delay := time.Duration(float64(BaseDelay) * math.Pow(2, float64(attempt)))
	if delay > MaxDelay {
		delay = MaxDelay
	}
	delay += jitter(delay)
	if delay < 0 {
		delay = 0
	}
	if delay > MaxDelay {
		delay = MaxDelay
	}
	return delay
}

// parseRetryAfter parses an integer-seconds Retry-After header, capped at
// MaxDelay. It reports false for absent or malformed values.
func parseRetryAfter(value string) (time.Duration, bool) {
	if value == "" {
		return 0, false
	}
	seconds, err := strconv.Atoi(value)
	if err != nil || seconds <= 0 {
		return 0, false
	}
	// Clamp before converting: a huge Retry-After (for example
	// 10000000000) overflows time.Duration when multiplied by
	// time.Second and wraps negative, which would bypass the MaxDelay
	// clamp below and produce immediate retries.
	if maxSeconds := int(MaxDelay / time.Second); seconds > maxSeconds {
		seconds = maxSeconds
	}
	delay := time.Duration(seconds) * time.Second
	if delay > MaxDelay {
		delay = MaxDelay
	}
	return delay, true
}

// jitter returns a random offset in [-25%, +25%] of delay.
func jitter(delay time.Duration) time.Duration {
	if delay <= 0 {
		return 0
	}
	span := int64(float64(delay) * jitterFraction)
	if span <= 0 {
		return 0
	}
	return time.Duration(rand.Int64N(span*2) - span)
}

// Wait sleeps for delay unless the context is canceled first. A canceled
// context returns promptly with the context error (FR-013).
func Wait(ctx context.Context, delay time.Duration) error {
	if delay <= 0 {
		return ctx.Err()
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
