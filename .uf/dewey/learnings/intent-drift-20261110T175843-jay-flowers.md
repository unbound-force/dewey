---
tag: intent-drift
author: jay-flowers
category: gotcha
created_at: 2026-11-10T17:58:43Z
identity: intent-drift-20261110T175843-jay-flowers
tier: draft
---

When a change scopes a contract change to a subset of tools that share a helper, audit every call site of that helper before writing non-goals. Enrichment added inside a shared path reaches all sibling tools: dewey's toSemanticResults (tools/semantic.go) is called by semantic_search (:75), similar (:113), and semantic_search_filtered (:249), so validity/as_of enrichment added for the two semantic search tools (validity-aware-retrieval, dewey #148 / PR #160) also reached similar — whose enrichment the proposal had just declared a non-goal, on the false premise that similar results are "block hits without page metadata" (they already carried tier/category/created_at through the same helper). When the misalignment surfaces, prefer amending the spec to document delivered shared-helper behavior plus a pinning test over adding a flag parameter whose sole purpose is suppressing provenance metadata; validate any non-goal premised on sibling tools' supposed lack of metadata against the helper's call graph before freezing it.
