---
tag: test-quality
author: jay-flowers
category: pattern
created_at: 2026-11-09T01:49:20Z
identity: test-quality-20261109T014920-jay-flowers
tier: draft
---

When a function aggregates or dedupes items by a key (page name, identity, request id) and reports a count of distinct keys, single-item-per-key test fixtures cannot distinguish distinct-key counting from raw item counting. Cover the aggregation unit with a fixture carrying at least two items sharing one key and assert the distinct-key count. Example (dewey PR #159): fixAnchorDrift dedupes anchor_drift findings by page and reports learnings marked; a fixture with one learning citing two drifted paths must assert fixed.anchor_drift == 1, because the existing one-anchor-per-learning fixtures pass even if the dedup is removed.
