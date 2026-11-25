---
tag: deweystore-boundaries
author: jay-flowers
category: gotcha
created_at: 2026-11-11T02:12:16Z
identity: deweystore-boundaries-20261111T021216-jay-flowers
tier: draft
---

A lower-layer package must not define its documented behavior by reference to an upper layer's unexported helper. store.ClaimSupersession's GoDoc said it decodes "with tools.decodeLearningProperties' semantics" while tools imports store, and it re-implemented the empty-text/JSON-null-to-empty-map rule inline. Same-semantics copies on two code paths are exactly what a two-path convergence fix should remove; extract the rule into the lower layer and have the upper layer delegate.
