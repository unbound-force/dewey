---
tag: test-quality
author: jay-flowers
category: gotcha
created_at: 2026-11-09T02:03:25Z
identity: test-quality-20261109T020325-jay-flowers
tier: draft
---

Single-element fixtures cannot distinguish per-aggregate from per-element result cardinality. When a spec mandates aggregate cardinality — e.g. dewey's lint anchor checks (issue #147): an unknown as_of pin MUST yield exactly one anchor_missing finding per learning, not one per evidence locator — the fixture needs at least two elements so the asserted total changes under the wrong cardinality. In PR #159's TestLint_AnchorChecks, the lost-pin learning carried one evidence path, so per-learning and per-anchor counting were indistinguishable (both yield 3 total findings and anchor_missing == 2); adding a second same-repo path makes the existing len(got) == 3 assertion discriminating (a per-anchor regression would yield 4). Rule of thumb: when a contract says "exactly N per aggregate", give the aggregate at least N+1 elements or assert the per-aggregate count explicitly.
