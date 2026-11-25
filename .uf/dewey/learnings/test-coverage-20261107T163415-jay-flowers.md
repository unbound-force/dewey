---
tag: test-coverage
author: jay-flowers
category: gotcha
created_at: 2026-11-07T16:34:15Z
identity: test-coverage-20261107T163415-jay-flowers
tier: draft
---

Multi-pass search fill logic (exact pass, then expanded/fallback pass) has a cap seam that only a binding-direction test holds: a case where fallback candidates outnumber the remaining slots (limit minus first-pass results). An early-exit test (first pass fills the limit) never reaches the fallback cap, and an open-slots test (fallback matches fewer than the remaining slots) passes even when the cap is passed the wrong value or dropped entirely — the suite stays green while the result-size contract regresses (up to 2x limit results). Found in dewey PR #157 (search-compound-terms): Search("foo-bar", 1) exits at the early-return guard and Search("foo-bar", 20) leaves cap slack, so the limit-len(results) argument at search_index.go:99 was unpinned. When writing or reviewing multi-pass fill code, always add the boundary case: firstPassResults < limit AND fallbackMatches > limit - firstPassResults. Same family as learning test-coverage-20261103T185329-jay-flowers (spec bounds no test pins are contracts CI cannot guard).
