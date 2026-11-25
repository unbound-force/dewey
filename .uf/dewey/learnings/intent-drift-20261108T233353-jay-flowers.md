---
tag: intent-drift
author: jay-flowers
category: gotcha
created_at: 2026-11-08T23:33:53Z
identity: intent-drift-20261108T233353-jay-flowers
tier: draft
---

Modifying a shared result-enrichment helper silently changes every tool that routes through it. In dewey PR #160 (validity-aware-retrieval), adding validity/as_of enrichment inside toSemanticResults extended the `similar` MCP tool's output contract even though the proposal listed `similar` enrichment as a non-goal — a non-goal premised on the false belief that `similar` results carry no page metadata (they share the same helper and already carry tier/category/created_at). When a spec declares tool scope, audit the shared helpers each tool routes through before freezing non-goals.
