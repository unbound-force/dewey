---
tag: test-determinism
author: jay-flowers
category: gotcha
created_at: 2026-11-07T17:06:43Z
identity: test-determinism-20261107T170643-jay-flowers
tier: draft
---

dewey's vault SearchIndex returns results in posting-list order with no ranking applied: index is map[string][]blockRef (insertion-ordered slices), matchAllLocked iterates the rarest term's candidates in insertion order, and SortByRelevance exists but is documented unused. Consequently, search-index test fixtures that build blocks by ranging over a Go map randomize result order per run — count and membership assertions stay green but order assertions flake. Build search-index fixtures from ordered slice literals (the file's established pattern) so current and future order assertions remain deterministic. Flagged on dewey PR #157 (divisor-testing, thread PRRT_kwDORhvIwc604MM-, compoundIndex at vault/search_index_test.go:176).
