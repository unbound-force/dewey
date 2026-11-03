package openaicompat

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"
)

// TestParseRetryAfter_HugeValueClamped pins the int64-overflow guard: a
// giant Retry-After clamps to MaxDelay instead of wrapping negative and
// bypassing the cap (code review L-1).
func TestParseRetryAfter_HugeValueClamped(t *testing.T) {
	delay, ok := parseRetryAfter("10000000000")
	if !ok {
		t.Fatal("expected huge Retry-After to parse as present")
	}
	if delay != MaxDelay {
		t.Fatalf("huge Retry-After: got %v, want MaxDelay (%v)", delay, MaxDelay)
	}
}

// TestParseRetryAfter_MalformedTreatedAsAbsent pins FR-011: missing,
// negative, zero, and non-integer values report absent so exponential
// backoff applies.
func TestParseRetryAfter_MalformedTreatedAsAbsent(t *testing.T) {
	for _, value := range []string{"", "-5", "0", "abc", "1.5"} {
		if _, ok := parseRetryAfter(value); ok {
			t.Fatalf("malformed Retry-After %q: expected absent", value)
		}
	}
}

// TestParseRetryAfter_HonoredWithinCap pins that a positive
// integer-seconds value within the cap is honored verbatim (FR-011).
func TestParseRetryAfter_HonoredWithinCap(t *testing.T) {
	delay, ok := parseRetryAfter("7")
	if !ok {
		t.Fatal("expected integer Retry-After to parse as present")
	}
	if delay != 7*time.Second {
		t.Fatalf("Retry-After 7: got %v, want 7s", delay)
	}
}

// TestRetryDelay_ExponentialBounds pins the FR-011 exponential branch:
// with no Retry-After, the delay is BaseDelay doubled per attempt with
// plus-or-minus 25 percent jitter, clamped to MaxDelay. The jitter
// widens the inclusive window to [0.75*2^attempt, 1.25*2^attempt]
// seconds; once the raw delay reaches the cap the window clamps to
// [45s, 60s] (jitter applies to the clamped MaxDelay and re-clamps
// from above). The retry loop only reaches attempt MaxRetries, but
// direct calls exercise the cap clauses too.
func TestRetryDelay_ExponentialBounds(t *testing.T) {
	for attempt := 0; attempt <= 8; attempt++ {
		raw := time.Duration(float64(BaseDelay) * math.Pow(2, float64(attempt)))
		lower, upper := raw-raw/4, raw+raw/4
		if raw >= MaxDelay {
			lower, upper = MaxDelay-MaxDelay/4, MaxDelay
		}
		for sample := 0; sample < 50; sample++ {
			got := RetryDelay(attempt, "")
			if got < lower || got > upper {
				t.Fatalf("attempt %d: delay %v outside inclusive bounds [%v, %v]", attempt, got, lower, upper)
			}
		}
	}
}

// TestWait_CanceledContext pins FR-013: a canceled context makes Wait
// return the context error promptly instead of sleeping the delay.
func TestWait_CanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	start := time.Now()
	err := Wait(ctx, time.Minute)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("unexpected error: %v", err)
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("canceled context must return promptly, took %v", elapsed)
	}
}

// TestWait_ElapsesDelay pins that Wait sleeps the requested delay and
// returns nil on a live context.
func TestWait_ElapsesDelay(t *testing.T) {
	start := time.Now()
	if err := Wait(context.Background(), 5*time.Millisecond); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if elapsed := time.Since(start); elapsed < 5*time.Millisecond {
		t.Fatalf("delay not elapsed: %v", elapsed)
	}
}

// TestWait_NonPositiveDelay pins the delay<=0 branch: Wait defers to
// the context immediately rather than arming a timer, so a live
// context returns nil.
func TestWait_NonPositiveDelay(t *testing.T) {
	if err := Wait(context.Background(), 0); err != nil {
		t.Fatalf("live context with zero delay: unexpected error %v", err)
	}
	if err := Wait(context.Background(), -time.Second); err != nil {
		t.Fatalf("live context with negative delay: unexpected error %v", err)
	}
}
