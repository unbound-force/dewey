---
tag: concurrency-testing
author: jay-flowers
category: gotcha
created_at: 2026-11-11T00:05:19Z
identity: concurrency-testing-20261111T000519-jay-flowers
tier: draft
---

When a regression test drives N concurrent StoreLearning calls at the same supersession target, a closed-channel start barrier does not make the per-call outcome deterministic: each call validates at the start and returns an 'already superseded by' error if another call's claim landed first, so losers can be validation errors rather than warned writers and their successor pages may not exist. Assert the store-level invariant (exactly one superseded_by) and allow each loser to be either winner-warned or validation-errored, or add a seam that releases validation-completion before any claim runs. The CAS makes the store claim deterministic, not the classification of each call's result.
