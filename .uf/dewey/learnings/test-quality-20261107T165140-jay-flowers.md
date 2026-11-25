---
tag: test-quality
author: jay-flowers
category: gotcha
created_at: 2026-11-07T16:51:40Z
identity: test-quality-20261107T165140-jay-flowers
tier: draft
---

Closed-store error injection fails every SQLite query uniformly, so it cannot cover partial-failure branches in multi-lookup fallback chains: vault findStorePage's prefix loop (vault/vault_store.go:520-525) needs exact-lookup-miss plus prefixed-lookup-error, a split outcome a closed *store.Store cannot produce because all queries route through the same db.QueryRow. When the unreachable branch mirrors an already-tested guard line-for-line, document the coverage limitation at the error test (the suite already notes the uniform-failure semantics at vault/vault_store_test.go:1785) rather than introducing an interface fault-injection seam solely for branch coverage — a seam is a production structural change that must justify itself behaviorally, not by coverage percentage alone.
