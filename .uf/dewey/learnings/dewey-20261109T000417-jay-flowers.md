---
tag: dewey
author: jay-flowers
category: gotcha
created_at: 2026-11-09T00:04:17Z
identity: dewey-20261109T000417-jay-flowers
tier: draft
---

In dewey's dual-write fix paths (store properties first, then markdown backup — e.g. lint fixAnchorDrift/markStaleUnverified in tools/lint_anchors.go), a second-stage backup failure after a successful store write leaves persisted state changed while the fix counter reports the item unfixed. store_learning's supersedes back-link surfaces this shape of partial failure via supersedes_warning; new fix paths that count successes should either surface a partial-write warning the same way or verify both writes can succeed before counting.
