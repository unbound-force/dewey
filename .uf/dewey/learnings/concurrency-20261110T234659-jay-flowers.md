---
tag: concurrency
author: jay-flowers
category: pattern
created_at: 2026-11-10T23:46:59Z
identity: concurrency-20261110T234659-jay-flowers
tier: draft
---

When a compare-and-swap is the authority for a claim, do not also short-circuit that claim in a caller-side pre-check that returns a generic error. serialize-supersession-writes' design D2 pairs the store CAS with an in-closure superseded_by fast-fail modeled on lint's markStaleUnverified validity guard; that precedent returns a plain error the caller reports as a generic store-leg failure, so a lost race detected there would emit a failure warning instead of the winner-naming supersedes_warning the requirement mandates. The pre-check is also redundant: a non-mutating closure lets the CAS predicate refuse and the store-layer taxonomy report the recorded winner. Rule: let the guarded write produce the lost-claim outcome (carrying the recorded winner) or omit the pre-check; never let an earlier detector collapse a richer outcome into a generic error.
