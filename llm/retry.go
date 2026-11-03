package llm

import (
	"math"
	"math/rand/v2"
	"strconv"
	"time"
)

// synthRetryDelay computes a retry delay shared by the networked providers.
// A valid integer-seconds Retry-After value is honored (capped at maxDelay).
// Otherwise exponential backoff with ±25% jitter is applied. The jitter
// desynchronizes concurrent retries.
func synthRetryDelay(attempt int, retryAfter string, baseDelay, maxDelay time.Duration) time.Duration {
	if retryAfter != "" {
		if secs, err := strconv.Atoi(retryAfter); err == nil && secs >= 0 {
			// Clamp before multiplying: a large Retry-After value would
			// overflow time.Duration, wrap negative, bypass the cap below,
			// and make time.After fire immediately.
			if secs > int(maxDelay/time.Second) {
				return maxDelay
			}
			d := time.Duration(secs) * time.Second
			if d > maxDelay {
				d = maxDelay
			}
			return d
		}
	}
	d := time.Duration(float64(baseDelay) * math.Pow(2, float64(attempt)))
	if d > maxDelay {
		d = maxDelay
	}
	if d < 2 {
		// rand.Int64N requires n > 0; tiny test-configured delays need no
		// jitter.
		return d
	}
	jitter := time.Duration(rand.Int64N(int64(d)/2)) - d/4
	d += jitter
	if d < 0 {
		d = 0
	}
	if d > maxDelay {
		d = maxDelay
	}
	return d
}
