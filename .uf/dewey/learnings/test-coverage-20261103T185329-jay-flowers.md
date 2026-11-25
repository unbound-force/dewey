---
tag: test-coverage
author: jay-flowers
category: pattern
created_at: 2026-11-03T18:53:29Z
identity: test-coverage-20261103T185329-jay-flowers
tier: draft
---

When a spec mandates a set of numeric bounds (dewey spec 017 FR-025: embedding 30s, synthesis 300s, probe 1s), a test that pins only the values reachable through one seam (the per-client httpClient.Timeout fields) silently drops bounds held as package-level constants consumed deeper in the call path. In PR #144's openaicompat package, TestClient_CapabilityTimeouts pinned the two client-field timeouts but omitted probeTimeout — the one constant that bounds write-mutex hold time in Available() and backs SC-004's one-second actionable-error outcome; no test would fail if the constant regressed. Pattern: when writing a capability/contract test, enumerate every spec-mandated numeric bound for the changed component and assert each where it lives, including package-level constants, not just the values exposed on the constructed object. A spec bound no test pins is a contract CI cannot guard. Same family as learning go-testing-20261102T221804 (contracts reachable only through unexercised seams go silently untested).
