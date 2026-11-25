---
tag: spec-alignment
author: jay-flowers
category: gotcha
created_at: 2026-11-09T01:42:26Z
identity: spec-alignment-20261109T014226-jay-flowers
tier: draft
---

dewey PR #159 (lint anchor checks, guard feedback on learningAnchors): the implementation folded an extra eligibility condition — at least one same-repo <repo>:<path> evidence locator — into the same boolean that encoded the spec-stated conditions (valid hex as_of pin, non-superseded/historical validity). The spec delta's unknown-pin anchor_missing requirement was unconditional for eligible learnings, so learnings citing only repo#<n>, session:, learning/, or other-repo locators silently escaped pin verification, and the spec delta, design, and README shipped disagreeing with the code inside one PR. Pattern to apply: when implementing an OpenSpec delta, map each spec-stated eligibility condition to its own traceable gate; any implementation-motivated extra condition (here: skipping a git subprocess for zero-anchor learnings) must be written into the delta first or dropped — burying it inside a composite ok return makes the narrowing invisible to scenario-level review.
