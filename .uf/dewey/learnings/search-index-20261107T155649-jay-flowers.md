---
tag: search-index
author: jay-flowers
category: gotcha
created_at: 2026-11-07T15:56:49Z
identity: search-index-20261107T155649-jay-flowers
tier: draft
---

Broadening an index's query expansion interacts with downstream result merges. In dewey, SearchIndex.Search's component fill pass consumes the result limit, and Client.FullTextSearch appends store-only LIKE hits only when in-memory hits fall short of the limit — so component matches can displace store-only exact-substring hits that previously surfaced for truncated-ID queries. Exact-first guarantees must be evaluated at the merged-output level, not just within one index.
