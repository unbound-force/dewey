---
tag: test-quality
author: jay-flowers
category: pattern
created_at: 2026-11-11T03:41:11Z
identity: test-quality-20261111T034111-jay-flowers
tier: draft
---

To deterministically test a check-then-act lost-claim window (validation read shared state clean; a concurrent writer mutates it before the write leg), interpose the racing write through a callback hook on a dependency the production flow already invokes inside the window — dewey's hookEmbedder fires Store.ClaimSupersession from inside EmbedBatch, which StoreLearning calls after validation resolves the supersession target and before the supersession store leg runs. The racing write executes on the same goroutine as the call under test, so the window hit is guaranteed with no goroutines, no sleeps, and no race-detector exposure. Complement it with a closed-channel-barrier goroutine race (N writers, one target) that asserts only the deterministic store-level invariant — exactly one claim lands, by compare-and-swap — and accepts every legitimate loser outcome class (warned write-leg loser, validation-error loser), because per-call classification races validation and any stronger assertion is flaky by construction. Document the determinism reasoning in the test comment so later readers do not "strengthen" the assertions back into flakiness.
