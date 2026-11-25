---
tag: test-quality
author: jay-flowers
category: gotcha
created_at: 2026-11-03T18:10:23Z
identity: test-quality-20261103T181023-jay-flowers
tier: draft
---

A tasks.md test task whose checkbox is flipped to [x] can still be undelivered: verify the clause-to-test mapping, not just suite-green. In dewey spec 017, T011 enumerated five retry-test clauses — including "exponential growth with jitter bounds" and "prompt context cancellation during a wait" — and was marked complete while those two clauses had no corresponding test. The suite stayed green because the untested behavior was executed through a stubbed seam (a no-op waitFn) that discarded its outputs, and the cancellation test exercised a different path (pre-request transport failure) than its name claimed. Before marking a test task complete, enumerate its clauses and point each at the concrete test function that asserts it; a green suite proves execution, not verification.
