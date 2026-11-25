---
tag: full-text-search
author: jay-flowers
category: pattern
created_at: 2026-11-07T19:41:51Z
identity: full-text-search-20261107T194151-jay-flowers
tier: draft
---

Two-pass search retrieval (exact-term pass, then lower-precision component/fuzzy fills) should gate any downstream merge with auxiliary sources on the pass-one exact-match count, not the total hit count. In dewey's vault.FullTextSearch (search-compound-terms, PR #157), component fills can saturate the result limit on their own; gating the SQLite store merge on the exact count and interleaving store-only exact-substring hits ahead of fills guarantees a store-only page holding the full compound ID is never displaced by weaker component matches. The index Search returns (results, exactCount) so the caller can slice hits[:exact] as the high-precision prefix. Regression test: TestFullTextSearch_ComponentFillsDoNotDisplaceStoreExactID. General rule: when a retrieval pipeline has precision tiers, propagate each tier's boundary count to merge points so higher-precision sources keep ranking priority under limit pressure.
