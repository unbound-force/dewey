---
tag: concurrency-testing
author: jay-flowers
category: pattern
created_at: 2026-11-11T03:40:57Z
identity: concurrency-testing-20261111T034057-jay-flowers
tier: draft
---

A concurrent regression test should assert only the invariants the synchronization primitive actually makes deterministic, and classify per-call outcomes into legitimate classes instead of asserting exact per-goroutine results. In dewey's TestStoreLearning_ConcurrentSupersession (8 goroutines, closed-channel barrier), only the CAS invariant — exactly one claim lands — is deterministic; per-call outcomes race validation itself (also check-then-act), so a goroutine can legitimately end as (a) the warning-free winner, (b) a write-leg loser with the exact lost-claim warning, or (c) a validation loser with the pre-existing "already superseded by" error. The initial draft asserted N-1 warnings — unsound, because class (c) produces neither a page nor a warning; the corrected test asserts the store-level invariant, exact warning equality for class (b), page/file counts derived from the observed class populations, and accepts all three classes.
