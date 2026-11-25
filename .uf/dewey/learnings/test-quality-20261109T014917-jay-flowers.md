---
tag: test-quality
author: jay-flowers
category: pattern
created_at: 2026-11-09T01:49:17Z
identity: test-quality-20261109T014917-jay-flowers
tier: draft
---

Dedup/aggregation units need multi-item-per-entity fixtures to be testable. When a function dedupes findings by entity and reports a distinct-entity count (dewey's fixAnchorDrift returns learnings marked, not findings processed), a fixture with one finding per entity returns the same number under both implementations, so a dedup regression passes silently. Pin the contract with at least one entity carrying multiple findings: a learning citing two drifted paths must assert fixed.anchor_drift == 1 while summary.anchor_drift == 2. Found in dewey PR #159 feedback item 11 — every lint-anchor fixture gave each learning at most one drifted anchor, leaving the seen[f.Page] dedup untested.
