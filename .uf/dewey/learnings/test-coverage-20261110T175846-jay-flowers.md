---
tag: test-coverage
author: jay-flowers
category: gotcha
created_at: 2026-11-10T17:58:46Z
identity: test-coverage-20261110T175846-jay-flowers
tier: draft
---

When additive JSON fields rely on `omitempty` for output stability, pin the absence case: assert the fields are absent for entities lacking the underlying data. Positive-path fixtures alone (PR #160's newValidityStore creates only validity-carrying pages) let a regression to always-present or default-valued fields pass the suite silently while changing every consumer's payload. Same family as learning test-quality-20261105T162539 (contract fields need presence+value assertions on the branch that produces them): the absence branch needs its own assertion.
