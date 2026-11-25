---
tag: retry-backoff
author: jay-flowers
category: gotcha
created_at: 2026-11-02T22:17:51Z
identity: retry-backoff-20261102T221751-jay-flowers
tier: draft
---

Exported delay/backoff helpers that exponentiate a base delay by an attempt counter must bound the attempt parameter (or clamp the exponent result) before converting float64 to time.Duration. Out-of-range float64→int64 conversion is implementation-defined per the Go spec (amd64 CVTTSD2SI yields min-int64), which is negative — it bypasses `delay > MaxDelay` upper-bound clamps and, after a `delay < 0 → 0` defensive guard, silently yields an immediate retry. Observed reviewing dewey openaicompat.RetryDelay (iteration 2 of spec 017): parseRetryAfter had been hardened against exactly this int64 overflow for huge Retry-After values, but the sibling exponential path retained it because the in-repo caller caps attempt at MaxRetries=5, leaving the overflow reachable only through the exported surface.
