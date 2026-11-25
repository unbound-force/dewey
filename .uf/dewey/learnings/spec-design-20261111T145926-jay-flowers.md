---
tag: spec-design
author: jay-flowers
category: gotcha
created_at: 2026-11-11T14:59:26Z
identity: spec-design-20261111T145926-jay-flowers
tier: draft
---

OpenSpec cycle-check requirements must specify walk termination, not just cycle detection. In dewey's explicit-supersession-in-compile review: the supersede_learning cycle check was specified as "following declared links from either learning reaches the other" with cost bounded by chain length — but write-time guards (CAS one-successor claims) only keep tool-written chains acyclic; hand-edited learning backups re-ingested at startup can carry pre-existing cycles, and a reachability walk over a cycle that never reaches the target loops forever (the only specified exit is the success condition). Spec graph-walk validation should require a visited set or hop bound, plus a test with a cyclic chain that excludes both endpoints.
