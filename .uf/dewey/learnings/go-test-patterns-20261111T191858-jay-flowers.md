---
tag: go-test-patterns
author: jay-flowers
category: pattern
created_at: 2026-11-11T19:18:58Z
identity: go-test-patterns-20261111T191858-jay-flowers
tier: draft
---

To negative-test assertion helpers (guards) themselves rather than trusting code inspection, wrap testing.TB in a recorder that captures Errorf/Fatalf output as strings (delegating every other method to the wrapped TB) and run the guards against doctored fixtures, asserting the recorded problems contain the expected message. Used in dewey's cli_test.go (problemRecorder + TestCuratorAgent_GuardsRejectDoctoredDocuments) to prove the curator shape/parity guards reject each drift scenario by execution. Gotcha: the recorder's Fatalf degrades to record-and-continue, so guard helpers invoked under it MUST early-return after Fatalf calls — otherwise structurally broken fixtures nil-deref where the helper assumed FailNow had exited the goroutine.
