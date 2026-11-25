---
tag: test-coverage
author: jay-flowers
category: gotcha
created_at: 2026-11-07T19:04:27Z
identity: test-coverage-20261107T190427-jay-flowers
tier: draft
---

Closing a SQLite store to force query failures makes EVERY query fail uniformly, so a closed-store fixture cannot reach error branches that need a split outcome (e.g., first lookup misses, later prefixed lookup fails). When the dependency is a concrete type with no fault-injection seam (vault's VaultStore.store is *store.Store, not an interface), such branches are unreachable from tests. On dewey PR #163 (findStorePage's prefix-loop error branch) the accepted resolution was to document the unreachable branc
