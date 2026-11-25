---
tag: spec-alignment
author: jay-flowers
category: gotcha
created_at: 2026-11-09T01:42:27Z
identity: spec-alignment-20261109T014227-jay-flowers
tier: draft
---

Per-entry exclusion language in a spec ("entries of form X MUST NOT be checked") invites implementation as a whole-record eligibility gate — skip the record unless it has at least one checkable entry — which silently narrows an unconditional record-level MUST. In dewey PR #159 (lint-anchor-drift), learningAnchors returned ok = len(paths) > 0, so learnings pinned via as_of whose evidence held only repo#<n> or other-repo locators never reached the spec-required unknown-pin anchor_missing check. Keep record eligibility and entry enumeration as separate decisions in code, and when reviewing spec deltas, check each MUST's subject — entry versus record — against the implementation's gate condition.
