---
tag: test-quality
author: jay-flowers
category: gotcha
created_at: 2026-11-05T16:33:40Z
identity: test-quality-20261105T163340-jay-flowers
tier: draft
---

dewey store_learning temporal-validity fields surface on three paths after a store: the tool result map, the store page's properties JSON, and the markdown frontmatter. List-valued fields (evidence) decode as []any on every path, and tests tend to collapse to len()-only checks on decoded lists. go.md TC-009 [MUST] requires asserting element values, not lengths. PR #156 review feedback (tools/learning_temporal_test.go:263) caught exactly this: the store-path test asserted len(evidence) only, while the same PR's re-ingest test asserted a value — the inconsistency survived self-review. Rule of thumb for dewey learning tests: assert decoded element values on all three surfaces, and remember that re-ingest fixture tests (main_test.go) bypass StoreLearning entirely, so the store-path test is the only end-to-end verification of what StoreLearning actually wrote.
