---
tag: search-index
author: jay-flowers
category: pattern
created_at: 2026-11-07T15:56:46Z
identity: search-index-20261107T155646-jay-flowers
tier: draft
---

In vault/search_index.go, the compound-term search keeps indexing and query expansion symmetric by sharing one splitter: splitCompound is the single source of the component rule (split on -/_, keep parts >= 2 chars), consumed by withCompoundComponents at indexing time and expandCompoundTerms at query time. Any future change to the component rule must go through splitCompound only — duplicating the rule on either side would let the index and the query drift apart. Cross-pass result ordering is structural, no
