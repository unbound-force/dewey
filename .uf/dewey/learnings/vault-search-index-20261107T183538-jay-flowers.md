---
tag: vault-search-index
author: jay-flowers
category: gotcha
created_at: 2026-11-07T18:35:38Z
identity: vault-search-index-20261107T183538-jay-flowers
tier: draft
---

SearchIndex result order within one pass is posting-list order — the match path performs no relevance sort (SortByRelevance exists but is unused), and BuildFrom iterates a map, so cross-page posting order is randomized per run. Tests that assert result ORDER must build single-page fixtures from an ordered block slice (see mkBlock/compoundIndex in vault/search_index_test.go, added for issue #149): within one page, posting lists follow the block-slice order, making same-pass order deterministic. Multi-page te
