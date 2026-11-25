---
tag: test-quality
author: jay-flowers
category: pattern
created_at: 2026-11-07T19:09:49Z
identity: test-quality-20261107T190949-jay-flowers
tier: draft
---

When a change adds a load-bearing return value consumed by a downstream gate — e.g. SearchIndex.Search's pass-one exact-match count, which both gates FullTextSearch's store merge (`exact >= limit`) and splits hits into exact vs component-fill portions (`hits[:exact]` / `hits[exact:]`) — add a direct unit assertion on that value in the same PR, inside the scenario tests that already exercise it. Discarding it at every call site (`results, _ :=`) leaves the documented GoDoc contract unasserted at the unit seam: end-to-end integration tests still catch corruption, but they report it as a hit-ordering mismatch several layers from the cause, costing real diagnosis time. Cheapest shape: capture the value where the scenario already runs and assert the expected count per query form (table-driven when forms differ, e.g. full ID → 1, truncated ID → 0, space-split → 1).
