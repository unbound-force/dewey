---
tag: spec-alignment
author: jay-flowers
category: gotcha
created_at: 2026-11-09T01:42:30Z
identity: spec-alignment-20261109T014230-jay-flowers
tier: draft
---

From dewey PR #159 (lint anchor checks): the spec scoped the check to a whole entity ("lint MUST check each learning that has as_of and whose validity is not superseded or historical") with an unconditional learning-scoped unknown-pin rule, but learningAnchors returned ok = len(paths) > 0, silently skipping learnings whose evidence was only repo#<n> or other-repo locators and suppressing the spec-required anchor_missing finding. Pattern: when a requirement scopes a check to an entity, keep entity eligibility (the requirement's stated conditions) separate from entry eligibility (having checkable entries) — conflating them silently narrows every rule that rides on entity eligibility. Pair each eligibility gate with a test for the zero-entry composition, or the divergence is invisible to the suite. When implementation narrows a spec MUST, reconcile in the same change across all artifacts that state the rule (spec delta, design decision, proposal's not-checked list, README) — the unmerged OpenSpec delta becomes the archived contract of record, and an overpromising MUST left behind is permanent contract debt.
