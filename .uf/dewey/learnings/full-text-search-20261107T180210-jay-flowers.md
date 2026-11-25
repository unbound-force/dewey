---
tag: full-text-search
author: jay-flowers
category: pattern
created_at: 2026-11-07T18:02:10Z
identity: full-text-search-20261107T180210-jay-flowers
tier: draft
---

When a search expansion pass (component fills, fuzzy fallback) can saturate the result limit, propagate the precision signal (the exact-match count) alongside the results and key downstream gates and slicing on it. In dewey's search-compound-terms fix, SearchIndex.Search now returns the pass-one exact-match count, and vault FullTextSearch gates its store merge on that count and interleaves store-only exact-substring hits ahead of pass-two component fills — so lower-precision expansion can never displace a higher-precision match (review-council on PR unbound-force/dewey#157, adversary finding).
