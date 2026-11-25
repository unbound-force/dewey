---
tag: test-design
author: jay-flowers
category: pattern
created_at: 2026-11-09T02:03:32Z
identity: test-design-20261109T020332-jay-flowers
tier: draft
---

A count assertion only discriminates cardinality rules when the fixture carries more than one child item. When a spec mandates per-entity emission ("one finding for the learning"), give the test fixture at least two child items (e.g., two evidence paths) so total-count and summary-count assertions fail under a per-item regression. In dewey PR #159, the unknown-pin anchor_missing test passed identically under per-learning and per-anchor implementations because the lost-pin learning cited a single path — presence checks never pin cardinality.
