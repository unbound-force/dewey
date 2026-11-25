---
tag: vault-testing
author: jay-flowers
category: gotcha
created_at: 2026-11-07T16:51:38Z
identity: vault-testing-20261107T165138-jay-flowers
tier: draft
---

In dewey's vault package, closing the store (s.Close()) is the established error-injection technique, but it fails every subsequent query on the sqlite handle atomically — it cannot produce a split outcome (clean miss on an early lookup, error on a later one) across the sequential lookups inside a multi-query helper like findStorePage (exact → compiled/<name> → learning/<name>). Error branches after the first query are therefore unreachable without interface indirection over the concrete *store.Store. Before adding such a seam, weigh the Zero-Waste Mandate and Constitution IV: when the untested branch line-for-line mirrors an already-tested guard, a single-consumer test-only abstraction is bloat, and forcing a specific internal lookup to fail verifies implementation ordering rather than observable behavior. At LOW severity the proportionate remedy is a test comment or review reply documenting the bounded risk.
