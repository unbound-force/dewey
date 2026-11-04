package llm

import (
	"testing"
	"time"
)

func TestSynthRetryDelay_RetryAfterOverflowClamped(t *testing.T) {
	const (
		baseDelay = 1 * time.Second
		maxDelay  = 60 * time.Second
	)

	// A Retry-After of MaxInt64 seconds would overflow time.Duration when
	// multiplied by time.Second, wrap negative, bypass the maxDelay clamp,
	// and make time.After fire immediately. It must clamp to maxDelay.
	d := synthRetryDelay(0, "9223372036854775807", baseDelay, maxDelay)
	if d != maxDelay {
		t.Errorf("delay with Retry-After=MaxInt64 = %v, want %v", d, maxDelay)
	}

	// Sanity: a normal value is still honored.
	if d := synthRetryDelay(0, "3", baseDelay, maxDelay); d != 3*time.Second {
		t.Errorf("delay with Retry-After=3 = %v, want 3s", d)
	}

	// A sub-second maxDelay leaves no whole second of headroom; any positive
	// Retry-After must clamp to maxDelay rather than overflow.
	if d := synthRetryDelay(0, "9223372036854775807", baseDelay, 500*time.Millisecond); d != 500*time.Millisecond {
		t.Errorf("delay with sub-second maxDelay = %v, want %v", d, 500*time.Millisecond)
	}
}

// TestSynthRetryDelay_ExponentialBackoffGrowth verifies the fallback backoff
// doubles per attempt and that jitter stays within ±25% of the unjittered
// value (council round-3 G10).
func TestSynthRetryDelay_ExponentialBackoffGrowth(t *testing.T) {
	const (
		baseDelay = 1 * time.Second
		maxDelay  = 60 * time.Second
	)

	// Attempts 0..5 stay under the 60s cap (1s << 5 = 32s), so each delay
	// is baseDelay << attempt with jitter of at most ±25%.
	for attempt := 0; attempt <= 5; attempt++ {
		want := baseDelay << attempt
		lo := want - want/4
		hi := want + want/4
		for i := 0; i < 25; i++ {
			if d := synthRetryDelay(attempt, "", baseDelay, maxDelay); d < lo || d > hi {
				t.Errorf("attempt %d delay = %v, want within [%v, %v]", attempt, d, lo, hi)
			}
		}
	}
}

// TestSynthRetryDelay_InvalidRetryAfterFallsBackToBackoff verifies that
// non-integer, negative, and fractional Retry-After values are ignored in
// favor of the exponential backoff, while a valid value is honored exactly
// without jitter (council round-3 G10).
func TestSynthRetryDelay_InvalidRetryAfterFallsBackToBackoff(t *testing.T) {
	const (
		baseDelay = 1 * time.Second
		maxDelay  = 60 * time.Second
	)

	// Attempt 2 backs off 4s with ±25% jitter: [3s, 5s].
	for _, header := range []string{"abc", "-5", "1.5"} {
		for i := 0; i < 25; i++ {
			if d := synthRetryDelay(2, header, baseDelay, maxDelay); d < 3*time.Second || d > 5*time.Second {
				t.Errorf("Retry-After %q delay = %v, want within [3s, 5s]", header, d)
			}
		}
	}

	// A valid Retry-After is honored exactly, without jitter.
	if d := synthRetryDelay(2, "7", baseDelay, maxDelay); d != 7*time.Second {
		t.Errorf("valid Retry-After delay = %v, want 7s", d)
	}
}

// TestSynthRetryDelay_CappedBackoffJitterBounds verifies that once the
// backoff clamps to maxDelay, the applied jitter stays within
// [maxDelay - maxDelay/4, maxDelay] (council round-3 G10).
func TestSynthRetryDelay_CappedBackoffJitterBounds(t *testing.T) {
	const (
		baseDelay = 1 * time.Second
		maxDelay  = 4 * time.Second
	)

	// Attempt 10 wants 1024s, clamped to 4s before jitter; jitter is
	// [-1s, +1s) and the re-clamp caps the result at 4s: [3s, 4s].
	for i := 0; i < 25; i++ {
		if d := synthRetryDelay(10, "", baseDelay, maxDelay); d < 3*time.Second || d > maxDelay {
			t.Errorf("capped delay = %v, want within [3s, %v]", d, maxDelay)
		}
	}
}
