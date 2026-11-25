---
tag: test-quality
author: jay-flowers
category: pattern
created_at: 2026-11-19T00:35:23Z
identity: test-quality-20261119T003523-jay-flowers
tier: draft
as_of: "dewey@50e8ff8ff6df4f2334c0dd2d67b92d99b27f7c2d"
evidence:
  - "dewey:.uf/artifacts/dispatch/4812b6f9-699e-424a-8783-32211cbb9053.json"
---

When an in-place refresh guard ORs multiple drift comparisons (dewey reIngestLearnings cheap path: existing.Properties != properties || existing.Category != fm.Category || existing.Tier != tier), give each arm its own regression test whose fixture differs ONLY in that field: every other field set to the exact file-derived value using the same derivation production applies (parseLearningFrontmatter → properties() → json.Marshal), and blocks given hand-set UUIDs so any block work breaks UUID identity. Because the guard is a disjunction, an unisolated fixture can pass through a different arm's refresh — a tier-drift test whose category also drifted would still pass if the tier arm were removed. Isolation makes mutation verification per-arm: removing any single comparison fails exactly one named test.
