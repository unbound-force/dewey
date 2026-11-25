---
tag: semantic-search
author: jay-flowers
category: gotcha
created_at: 2026-11-08T23:33:20Z
identity: semantic-search-20261108T233320-jay-flowers
tier: draft
---

Multiplicative re-ranking (score × weight) only preserves the intended demotion order for non-negative scores. Cosine similarity spans [-1, 1]: below zero, a 0.6 down-weight raises the product (-0.9 → -0.54) and inverts the demotion. In dewey's validity-aware retrieval (tools/semantic_validity.go), both ranked tools are guarded only implicitly — threshold <= 0 defaults to 0.3 and the store filters sim >= threshold, so rankByValidity never sees negative scores — while the `similar` tool queries SearchSimilar with an explicitly-zeroable threshold and can hit negative cosine similarities directly (PR #160 review, correlation 10f81d8d).
