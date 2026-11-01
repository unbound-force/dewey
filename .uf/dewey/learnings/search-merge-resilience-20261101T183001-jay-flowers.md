---
tag: search-merge-resilience
author: jay-flowers
category: gotcha
created_at: 2026-11-01T18:30:01Z
identity: search-merge-resilience-20261101T183001-jay-flowers
tier: draft
---

When merging a SQL LIKE-containment search into an in-memory tokenized search (dewey mcp-internal-store-access, vault FullTextSearch), gate the store merge on a non-blank query. The two search styles treat the empty query asymmetrically: the in-memory tokenizer produces zero terms and returns nothing, while LIKE '%' + escapeLike('') + '%' matches every row up to the limit. Without a TrimSpace gate, a store-backed client returns up to `limit` arbitrary blocks for an empty query while a store-less client returns none — breaking no-store parity (Constitution II) and leaking unrelated content. The same asymmetry applies to any future merge of set-matching SQL predicates into tokenized indexes: normalize the empty/blank input behavior at the merge boundary, not per-backend.
