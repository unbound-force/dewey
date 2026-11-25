---
tag: test-determinism
author: jay-flowers
category: gotcha
created_at: 2026-11-07T17:06:44Z
identity: test-determinism-20261107T170644-jay-flowers
tier: draft
---

Go test fixtures built by ranging over a map inject randomized iteration order into downstream structures. In dewey's vault package, compoundIndex (vault/search_index_test.go, PR #157) built page.blocks from a map[string]string, so search-index posting-list order varied per test run — order-insensitive assertions masked the problem, but any future order assertion would flake intermittently. Build indexed/searched fixtures from ordered slice literals (the pattern TestSearchIndex_BasicSearch uses) so index construction is deterministic; this both removes the latent flake and enables the full-order assertions that map-based fixtures force reviewers to weaken.
