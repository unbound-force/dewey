---
tag: race-test-design
author: jay-flowers
category: gotcha
created_at: 2026-11-11T03:09:45Z
identity: race-test-design-20261111T030945-jay-flowers
tier: draft
---

A concurrent race-regression test over a tool whose validation is check-then-act cannot assert per-call outcomes like 'the other N−1 calls carry the warning' or 'all N successors are stored': validation itself races the write, so a goroutine whose validation runs after another's claim lands takes the pre-existing validation-error path, writes nothing, and carries no warning — making those assertions intermittently false under -race. Assert only the invariant the atomic guard makes deterministic (exactly one claim lands; the recorded winner matches exactly one successor identity; every successful call kept its own claim), explicitly accept every legitimate loser class (warned write-leg loser, validation-error loser), and document in the test why the stronger assertions are unsound. Prescribing the unsound assertions in tasks.md/design.md and leaving them unamended after the correction creates an artifact that re-introduces the flake on re-implementation.
