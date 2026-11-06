---
tag: test-quality
author: jay-flowers
category: pattern
created_at: 2026-11-05T16:33:37Z
identity: test-quality-20261105T163337-jay-flowers
tier: draft
---

When a feature round-trips a field through multiple surfaces — the MCP result map, the store page's properties JSON, and the markdown backup file — the integration test must assert the field's specific values on every surface, not just presence or count. In dewey PR #156 (learning temporal validity), TestStoreLearning_TemporalFields asserted only len(evidence) == 2 on the store-properties surface and omitted evidence entirely from the result-map and frontmatter assertions, while the same PR's re-ingest test did assert evidence[1] — the write path's evidence values were the one unverified surface until review feedback caught it. Length-only checks pass on value corruption, reordering, and partial truncation; go.md TC-009 [MUST] requires asserting slice contents. Review heuristic: for each list-valued field a change introduces, enumerate every surface the field is written to and map each surface to a value assertion (not a length check) in the test that covers that path.
