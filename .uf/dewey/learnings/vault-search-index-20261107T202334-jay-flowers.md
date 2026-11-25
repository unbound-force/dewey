---
tag: vault-search-index
author: jay-flowers
category: pattern
created_at: 2026-11-07T20:23:34Z
identity: vault-search-index-20261107T202334-jay-flowers
tier: draft
---

SearchIndex posting lists are ordered []blockRef slices appended in block-indexing order, rarest-term tie-breaking keeps the first term in query order, and no relevance sort runs in the Search path — so same-pass result order is deterministic for an ordered block fixture. The compound-term tests in vault/search_index_test.go exploit this via compoundIndex(blocks...): a single-page map (BuildFrom's map iteration would randomize cross-page posting order) plus an ordered block slice. Order-asserting search-ind
