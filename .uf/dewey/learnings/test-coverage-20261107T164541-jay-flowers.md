---
tag: test-coverage
author: jay-flowers
category: pattern
created_at: 2026-11-07T16:45:41Z
identity: test-coverage-20261107T164541-jay-flowers
tier: draft
---

A table-driven test for a multi-key resolver (exact name, then prefixed variants) pins the lookup order only when a fixture makes two adjacent keys both hit. Add one coexistence fixture per adjacent precedence pair — e.g., a plain-named store page next to compiled/<same-name>, asserting the exact page wins — otherwise reordering the resolver passes the whole suite. Precedent: dewey's promote resolver tests (dewey-distill task 4.2) pin "namespace-beats-source-doc precedence" as a named case; the read-side findStorePage resolver added in PR #163 initially left exact-vs-prefix unpinned until review feedback (divisor-testing, item 6) requested the missing table row.
