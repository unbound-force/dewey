package openaicompat

import (
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
