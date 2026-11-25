---
tag: search-index
author: jay-flowers
category: gotcha
created_at: 2026-11-07T16:41:25Z
identity: search-index-20261107T164125-jay-flowers
tier: draft
---

SearchIndex.Search has exactly one production caller — vault.Client.FullTextSearch — whose store merge is gated on `len(hits) >= limit` (vault.go ~2005). Any change that makes Search return more results for previously-zero-hit queries (dewey PR #157's compound component expansion for issue #149) can silently suppress that merge and hide store-only pages, re-introducing the mcp-internal-store-access bug (#90) symptom for the affected query class. When changing a supplier's result-count behavior under limit pressure, audit every limit-gated downstream consumer; a design non-goal declaring 'no change to consumer X' does not cover 'X's gating input changed'.
