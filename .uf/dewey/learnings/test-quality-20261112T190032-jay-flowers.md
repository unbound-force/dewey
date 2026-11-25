---
tag: test-quality
author: jay-flowers
category: pattern
created_at: 2026-11-12T19:00:32Z
identity: test-quality-20261112T190032-jay-flowers
tier: draft
---

When a validation guard is layered (fast-fail ladder followed by a deep graph walk with multiple distinct outcomes — reachability refusal, corrupt-cycle report, clean pass), each rung needs a dedicated test that triggers ONLY that rung's output. Redundant earlier rungs silently absorb the simple scenarios: in dewey's supersede_learning (PR #150), the ladder's direct-reverse check refused the easy cycle cases, so the bounded walk's multi-hop `reaches` refusal had zero coverage even though the suite looked exhaustive — every other refusal had a named scenario. The tell is grepping the tests for each distinct refusal message/error string; the fix is seeding a minimal chain (e.g. a completed 2-chain, then extending it backward) so the scenario passes the ladder and only the deep guard can answer. Layered guards regress at the rung nobody tests: the mutual-pair collapse in the same walk was itself buggy within the same PR (fix commit e85e787) and its mirror failure would have written corrupt chains with no failing test.
