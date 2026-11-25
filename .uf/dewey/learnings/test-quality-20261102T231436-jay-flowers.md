---
tag: test-quality
author: jay-flowers
category: pattern
created_at: 2026-11-02T23:14:36Z
identity: test-quality-20261102T231436-jay-flowers
tier: draft
---

When tests inject a no-op sleep/wait function to keep retry tests fast, computed backoff delays are still produced but silently discarded. Capture the delay values passed to the injected wait (as openaicompat's TestClient_RetryThenSuccess does for the Retry-After path) and bound-assert them for the exponential path too — per-attempt inclusive ranges such as [0.75·2^attempt·base, 1.25·2^attempt·base], clamped at the max delay, keep jitter-bearing backoff contracts verifiable without flakiness.
