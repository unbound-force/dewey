---
tag: test-coverage
author: jay-flowers
category: gotcha
created_at: 2026-11-07T19:09:47Z
identity: test-coverage-20261107T190947-jay-flowers
tier: draft
---

When a change adds a return value to an exported method specifically to gate downstream behavior (dewey PR 157: SearchIndex.Search's pass-one exact-match count gates FullTextSearch's store merge), the same change's tests must assert that return value directly. Mechanically updating call sites with `results, _ :=` leaves the documented GoDoc contract enforced only through distant integration tests, where a corrupted value surfaces as a misleading failure far from its cause — an under-reported exact count presents as a store-merge limit failure in vault_test.go, not as a Search-contract failure. Assert the seam values directly (e.g., exact == 1 for full/space-split query forms, exact == 0 for the truncated form). Grounded in go.md TC-009 [MUST]: assert specific return values, not just the primary result.
