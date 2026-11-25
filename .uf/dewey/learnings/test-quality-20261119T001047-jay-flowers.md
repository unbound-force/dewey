---
tag: test-quality
author: jay-flowers
category: pattern
created_at: 2026-11-19T00:10:47Z
identity: test-quality-20261119T001047-jay-flowers
tier: draft
as_of: "dewey@50e8ff8ff6df4f2334c0dd2d67b92d99b27f7c2d"
evidence:
  - "dewey:.uf/artifacts/dispatch/ba50f507-f1e9-45af-9bca-a44a36fe7a81.json"
---

When a write path and a read path in different Go packages must agree on a derived value (e.g., a content hash), per-package tests that re-derive the other side's computation can silently mirror a bug in either derivation. Pin the contract with a joined seam test that runs the real writer against the real reader: in dewey's learning-reingest-on-file-change change, tools/learning_test.go re-implements main's splitFrontmatter derivation (BOM trim, SplitN on "---", TrimSpace, sha256[:8] hex), but the authoritative coverage is main_test.go's TestReIngestLearnings_ToolStoredLearningStaysCheap, which calls tools.StoreLearning (the production writer) and then reIngestLearnings (the production reader) in one test, asserting count 0 and zero embedder calls — so any divergence between the two derivations fails a test instead of causing a silent one-time re-embed of every stored item at startup. Prefer the joined test as the contract of record; keep the mirrored derivation only as a secondary localization aid.
