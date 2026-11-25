---
tag: full-text-search
author: jay-flowers
category: pattern
created_at: 2026-11-07T19:41:56Z
identity: full-text-search-20261107T194156-jay-flowers
tier: draft
---

When a search pipeline adds a lower-precision fallback pass (e.g., compound-term component matching) on top of a high-precision exact pass and then merges results with a second source (e.g., a persistent SQLite store), gate the merge on the high-precision pass's match count, not the total hit count, and interleave the second source's hits between the high-precision and fallback portions. Otherwise the fallback fills can saturate the result limit and displace higher-precision hits from the other source — a regression the high-precision pass alone never caused. Verified in dewey search-compound-terms (design D4): FullTextSearch gates on SearchIndex.Search's pass-one exact-match count so store-only pages holding a full compound ID rank ahead of component-only fills; regression test TestFullTextSearch_ComponentFillsDoNotDisplaceStoreExactID pins the ordering (vault/vault.go:2010-2057).
