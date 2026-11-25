---
tag: test-quality
author: jay-flowers
category: pattern
created_at: 2026-11-09T01:49:18Z
identity: test-quality-20261109T014918-jay-flowers
tier: draft
---

Result counters that dedupe by entity need a multi-finding-per-entity fixture. In dewey PR #159 (lint anchor checks), fixAnchorDrift counts distinct learning pages, not anchor_drift findings (design D4: fixed.anchor_drift = learnings marked), yet every test fixture gave each learning at most one drifted anchor — a count-findings-instead-of-entities regression or a double-mark regression would have passed the suite. Pin entity-deduped counters by asserting both sides: the raw finding count (e.g., summary.anchor_drift == 2), proving the fixture generates multiple findings for one entity, and the deduped fix count (fixed.anchor_drift == 1). Asserting only the deduped count lets a fixture regression satisfy the assertion vacuously.
