---
tag: test-quality
author: jay-flowers
category: gotcha
created_at: 2026-11-07T17:47:35Z
identity: test-quality-20261107T174735-jay-flowers
tier: draft
---

Boundary fixtures for count caps must be designed around WHERE the bound is enforced relative to collapsing transformations (trim/dedup). In dewey PR #156, normalizeEvidence enforces the 50-entry evidence cap on the raw input length BEFORE dedup (first statement of the function), so a test fixture of 51 IDENTICAL entries is the discriminating fixture: under the shipped pre-dedup cap it errors (51 > 50), while under a hypothetical dedup-then-cap regression it would collapse to 1 entry, return no error, and fail the test. A fixture of 51 DISTINCT entries errors under BOTH orderings and pins only the bound's existence, not its position — swapping identical for distinct entries weakens the test. A green expected-error test with N+1 identical entries is itself proof the bound fires pre-collapse. Before applying review feedback that "fixes" a fixture, re-derive the enforced ordering from the implementation and the spec: round-2 feedback claimed dedup-before-cap, while the code, the spec delta ("evidence MUST hold at most 50 entries"), and the design doc all specify or implement cap-first.
