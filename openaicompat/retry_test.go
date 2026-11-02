package openaicompat

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestRetryDelay_RetryAfter(t *testing.T) {
	tests := []struct {
		name       string
		retryAfter string
		want       time.Duration
	}{
		{name: "honored", retryAfter: "7", want: 7 * time.Second},
		{name: "capped", retryAfter: "120", want: MaxDelay},
		{name: "zero treated as absent", retryAfter: "0"},
		{name: "negative treated as absent", retryAfter: "-3"},
		{name: "non numeric treated as absent", retryAfter: "soon"},
		{name: "http date treated as absent", retryAfter: "Wed, 21 Oct 2015 07:28:00 GMT"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RetryDelay(0, tt.retryAfter)
			if tt.want > 0 {
				if got != tt.want {
					t.Fatalf("got %v, want %v", got, tt.want)
				}
				return
			}
			// Exponential fallback for attempt 0: 1s ±25%.
			assertWithinJitter(t, got, time.Second)
		})
	}
}

func TestRetryDelay_ExponentialGrowth(t *testing.T) {
	for attempt := 0; attempt <= MaxRetries; attempt++ {
		base := time.Duration(float64(BaseDelay) * pow2(attempt))
		if base > MaxDelay {
			base = MaxDelay
		}
		got := RetryDelay(attempt, "")
		if got < 0 {
			t.Fatalf("attempt %d produced negative delay %v", attempt, got)
		}
		assertWithinJitter(t, got, base)
		if got > MaxDelay {
			t.Fatalf("attempt %d exceeded the cap: %v", attempt, got)
		}
	}
}

func assertWithinJitter(t *testing.T, got, base time.Duration) {
	t.Helper()
	low := time.Duration(float64(base) * 0.75)
	high := time.Duration(float64(base) * 1.25)
	if got < low || got > high {
		t.Fatalf("delay %v outside jitter bounds [%v, %v]", got, low, high)
	}
}

func pow2(attempt int) float64 {
	result := 1.0
	for i := 0; i < attempt; i++ {
		result *= 2
	}
	return result
}

func TestWait_Cancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	start := time.Now()
	err := Wait(ctx, 5*time.Second)
	elapsed := time.Since(start)

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v, want context.Canceled", err)
	}
	if elapsed > time.Second {
		t.Fatalf("cancellation was not prompt: %v", elapsed)
	}
}

func TestWait_Elapses(t *testing.T) {
	ctx := context.Background()
	if err := Wait(ctx, time.Millisecond); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestWait_ZeroDelayReturnsImmediately(t *testing.T) {
	if err := Wait(context.Background(), 0); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
