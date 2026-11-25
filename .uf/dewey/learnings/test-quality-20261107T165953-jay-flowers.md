---
tag: test-quality
author: jay-flowers
category: pattern
created_at: 2026-11-07T16:59:53Z
identity: test-quality-20261107T165953-jay-flowers
tier: draft
---

Boundary tables for threshold filters need three cases, not two: all elements qualify, none qualify, and some qualify. The some-qualify (mixed) case is the most commonly omitted and is often where the semantics bite. In dewey PR #157 (issue #149), TestSplitCompound covered kb-review-adjud-01 (all parts >=2 chars) and a-b (no parts >=2 chars) but missed x-kb -> ["kb"] — the case where query expansion silently drops a constraint (the 1-char part) and broadens two-pass matching to any block containing the surviving component. When writing or reviewing a table-driven test for a filter, explicitly enumerate all/none/some; the mixed row is one line and pins the constraint-dropping semantics.
