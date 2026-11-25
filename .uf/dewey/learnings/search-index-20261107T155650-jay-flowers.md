---
tag: search-index
author: jay-flowers
category: pattern
created_at: 2026-11-07T15:56:50Z
identity: search-index-20261107T155650-jay-flowers
tier: draft
---

Two-pass exact-first query expansion preserves existing behavior by construction: run the unchanged exact-term AND query first; only when the query contains expandable terms AND exact matches under-fill the limit, run the expanded query with a UUID skip-set for dedupe. Every query that already fills the limit, or contains no expandable term, takes the identical single-pass path — which makes the change auditable against its spec (exact matches first, no duplicates, up to the limit).
