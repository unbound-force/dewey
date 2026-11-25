---
tag: test-quality
author: jay-flowers
category: gotcha
created_at: 2026-11-07T17:06:49Z
identity: test-quality-20261107T170649-jay-flowers
tier: draft
---

Go test fixtures that feed order-sensitive systems must be built from ordered slice literals, never by ranging over maps: Go randomizes map iteration order per run, so map-built fixtures make downstream ordering nondeterministic. The failure mode is indirect — existing order-insensitive assertions keep passing while the fixture silently caps assertion depth, and the first future order-sensitive assertion flakes intermittently. In dewey PR #157, vault/search_index_test.go's compoundIndex helper ranged over map[string]string; because posting lists are insertion-ordered slices (si.index map[string][]blockRef, refs appended in block order) and Search applies no relevance sort, same-pass result order equaled the randomized fixture order. Sibling reviewers consequently weakened ordering assertions to count + first-UUID. Converting the helper to ordered slice literals (matching TestSearchIndex_BasicSearch/Limit/ReindexPage) both restores the file pattern and makes deterministic full-order assertions possible (go.md TC-009). Audit any test helper that ranges over a map before using its output to build ordered structures.
