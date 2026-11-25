---
tag: go-slices
author: jay-flowers
category: gotcha
created_at: 2026-11-07T16:52:49Z
identity: go-slices-20261107T165249-jay-flowers
tier: draft
---

Lazy-copy Go helpers that return the input slice on the no-change path (as suggested for withCompoundComponents in dewey vault/search_index.go, PR #157) silently change the function's aliasing contract: callers that append to the result can write into the input's spare capacity when cap > len. The optimization is safe only when every caller passes an unretained fresh slice (e.g., an inline tokenize() result) under exclusive access. Before applying such a suggestion, grep for all call sites, verify none retain or share the input, and document the aliasing in the doc comment so future callers are warned.
