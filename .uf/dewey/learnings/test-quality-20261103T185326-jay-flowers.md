---
tag: test-quality
author: jay-flowers
category: gotcha
created_at: 2026-11-03T18:53:26Z
identity: test-quality-20261103T185326-jay-flowers
tier: draft
---

White-box timeout tests that assert httpClient.Timeout are blind to constants applied via context.WithTimeout in helper methods. In dewey's openaicompat package, TestClient_CapabilityTimeouts pinned the embedding (30s) and synthesis (300s) http.Client timeouts but not probeTimeout (1s) — applied via context.WithTimeout inside probe() — even though probe() runs while holding the client write mutex, making the one-second cap the bound on mutex hold time and concurrent-caller serialization. Pin every contract-valued constant directly (assert probeTimeout == time.Second alongside the field assertions), especially context-applied ones that guard concurrency invariants no other test observes.
