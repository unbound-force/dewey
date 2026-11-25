---
tag: concurrency-testing
author: jay-flowers
category: gotcha
created_at: 2026-11-11T00:05:24Z
identity: concurrency-testing-20261111T000524-jay-flowers
tier: draft
---

A concurrency regression test that asserts "exactly one winner and every other call carries a warning naming it" is unsound whenever the write path validates against the same state its write guards. In dewey's store_learning supersession, `prepareTemporal`/`resolveSupersedesTarget` read the target's `superseded_by` before any write and reject an already-superseded target; a goroutine released by a start barrier but scheduled after the first claim lands takes that validation-error path and produces neither a successor page nor a warning. Only "exactly one claim lands" is deterministic; the loser set splits into warned (write-leg loss) and errored (validation loss) subsets in a run-dependent ratio. Test designs must either arrange the spec's GIVEN (all calls pass validation) or accept both outcomes.
