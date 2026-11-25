---
tag: search-index
author: jay-flowers
category: pattern
created_at: 2026-11-07T18:02:12Z
identity: search-index-20261107T180212-jay-flowers
tier: draft
---

Index-side and query-side text expansion must share a single-source-of-truth grammar. Dewey's splitCompound is called both by withCompoundComponents (indexing) and expandCompoundTerms (query expansion), so the analyzer is symmetric by construction — a block indexed as kb-review-adjud-01 with components kb/review/adjud/01 is findable by any query form those expansions produce. Duplicating the splitting rule in two places would let index and query drift apart silently (review-council on PR unbound-force/dewey#157, architect finding).
