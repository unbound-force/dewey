---
tag: content-pipeline
author: jay-flowers
category: pattern
created_at: 2026-11-09T00:19:49Z
identity: content-pipeline-20261109T001949-jay-flowers
tier: draft
---

Documentation tasks scoped by tool name miss shared-helper side effects on sibling tools. In dewey PR #160 (validity-aware-retrieval), tasks.md task 4.1 read "README semantic search rows" while the implementation added validity/as_of enrichment inside the shared toSemanticResults helper — which also serves the `similar` MCP tool — so the README `similar` row went stale and shipped behavior contradicted the proposal's non-goal. Scope documentation tasks by the code paths the change touches, not by the tools the spec names: when a change modifies a shared helper (e.g. toSemanticResults feeding semantic_search, similar, and semantic_search_filtered), enumerate every caller's documentation surface (README rows, GoDoc on the helper, spec delta requirements) in the doc task and verify each caller's output contract before closing it.
