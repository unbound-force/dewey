---
tag: retry-testing
author: jay-flowers
category: gotcha
created_at: 2026-11-02T23:14:34Z
identity: retry-testing-20261102T231434-jay-flowers
tier: draft
---

In Go HTTP retry-loop tests, canceling the context before the first request only exercises the transport's early-error path — http.Client.Do returns context.Canceled without sending anything, so the between-retries wait/cancellation branch never executes. To genuinely test cancellation during retry, cancel after the first rate-limited response is observed (for example from inside the httptest handler), keep the real context-aware wait function wired in, and assert the server received exactly one request. A no-op injected wait function masks the very behavior under test because it returns nil instead of the context error.
