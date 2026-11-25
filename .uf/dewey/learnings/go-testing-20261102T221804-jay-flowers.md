---
tag: go-testing
author: jay-flowers
category: gotcha
created_at: 2026-11-02T22:18:04Z
identity: go-testing-20261102T221804-jay-flowers
tier: draft
---

Stubbing a time/wait seam (e.g. a waitFn sleep hook in an HTTP retry client) removes the only observation point for retry delay values and mid-wait cancellation. When a production function is reachable only through a stubbed seam, its contract goes silently untested: keep asserting the values passed to the stub (Retry-After honored, backoff within jitter bounds, delay capped), and unit-test the real wait function directly (canceled context must return the context error promptly). Observed in dewey spec 017 iteration 2: openaicompat.Wait and RetryDelay's exponential-backoff path had zero direct coverage because every test replaced waitFn — the spec's FR-013 wait-cancellation contract was only exercised on the request leg.
