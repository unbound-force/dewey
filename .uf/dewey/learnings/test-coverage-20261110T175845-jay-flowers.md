---
tag: test-coverage
author: jay-flowers
category: pattern
created_at: 2026-11-10T17:58:45Z
identity: test-coverage-20261110T175845-jay-flowers
tier: draft
---

When output fields are added inside a shared result converter (dewey's toSemanticResults feeds semantic_search, semantic_search_filtered, AND similar), the test plan must enumerate every caller of that converter: each affected tool gets a pinning test asserting the new fields' presence and value, or an explicit gate excluding it — otherwise the untargeted callers' contracts change with no test that can fail. In PR #160 the validity/as_of enrichment reached `similar` untested because the test task enumerated only the two tools the spec named. Test-side complement of learnings intent-drift-20261108T233353 and api-design-20261108T233345 (which cover the scope-statement side).
