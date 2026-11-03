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
