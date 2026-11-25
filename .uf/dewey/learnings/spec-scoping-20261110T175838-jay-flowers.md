---
tag: spec-scoping
author: jay-flowers
category: decision
created_at: 2026-11-10T17:58:38Z
identity: spec-scoping-20261110T175838-jay-flowers
tier: draft
---

When shared-helper enrichment extends a tool's output past a non-goal whose premise is false, and the OpenSpec artifacts are still in-PR and mutable, prefer amending the artifacts to match the established shared-helper behavior over gating the helper per caller. In dewey PR #160 (validity-aware-retrieval), the architect assessment recommended the reviewer's option (b): the 013-knowledge-compile precedent (tier/category/created_at enriched for all three semantic tools through toSemanticResults), Constitution III's provenance direction, and omitempty additive fields (byte-identical output for pages without validity metadata) all favor documenting and pinning the enrichment. A per-caller gate would create a two-tier enrichment regime in one helper — tier/category/created_at unconditional, validity/as_of caller-gated — a competing pattern for the same abstraction.
