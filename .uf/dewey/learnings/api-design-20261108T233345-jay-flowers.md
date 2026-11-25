---
tag: api-design
author: jay-flowers
category: gotcha
created_at: 2026-11-08T23:33:45Z
identity: api-design-20261108T233345-jay-flowers
tier: draft
---

Enrichment added to a shared result converter applies to every caller. tools/semantic.go's toSemanticResults feeds semantic_search, semantic_search_filtered, AND similar — so a field added there (validity/as_of in dewey PR #160) appears in `similar` results too, even when the proposal lists `similar` under "Not covered". Scope statements and non-goal lists must be written against shared code paths, not per tool, or they will misdescribe delivered behavior. Check every caller of a shared converter before writing scope statements.
