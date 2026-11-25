---
tag: search-merge
author: jay-flowers
category: pattern
created_at: 2026-11-07T18:35:34Z
identity: search-merge-20261107T183534-jay-flowers
tier: draft
---

When a ranked retrieval gains a fallback/fuzzy second pass and its results are merged with a secondary source, the merge must gate on the exact-pass count, not the total hit count. In dewey PR #157 (search-compound-terms, design D4), SearchIndex.Search returns (results, exactCount) so vault FullTextSearch gates its store merge on pass-one exact matches and interleaves store-only exact-substring hits ahead of pass-two component fills (merged = exact hits + store hits + fills, truncated to limit). Without the
