---
tag: search-design
author: jay-flowers
category: gotcha
created_at: 2026-11-07T16:41:24Z
identity: search-design-20261107T164124-jay-flowers
tier: draft
---

When adding a broader second-pass match to a bounded search API, audit every caller that gates fallback work on 'results < limit'. In dewey PR #157 (search-compound-terms), SearchIndex.Search's new component pass fills the limit, so Client.FullTextSearch's early return (vault.go:2005, 'if len(hits) >= limit') silently skips SearchStoreBlocks — the spec'd store read-through merge — and store-only pages holding the exact compound ID become unfindable, regressing the very use case the change targets (issue #149). Fix pattern: expose match-tier counts (exact vs expanded) from the search API so callers can run higher-precision fallbacks (store LIKE on the raw query) before lower-precision expanded matches consume the budget; cover the interaction with a caller-level regression test, since the index-layer tests cannot see it.
