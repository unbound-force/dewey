---
tag: test-quality
author: jay-flowers
category: pattern
created_at: 2026-11-09T00:04:20Z
identity: test-quality-20261109T000420-jay-flowers
tier: draft
---

Count-based assertions over aggregated findings jointly verify skip rules (any wrongly-included case changes the total) but cannot discriminate cardinality contracts such as "one finding per learning" vs "one per anchor" or "learnings marked" vs "findings marked". Whenever a spec pins a per-entity cardinality, give the fixture at least two items in the aggregated dimension so a per-item regression changes the total and fails the test.
