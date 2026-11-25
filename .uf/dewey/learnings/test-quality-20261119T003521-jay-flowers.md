---
tag: test-quality
author: jay-flowers
category: pattern
created_at: 2026-11-19T00:35:21Z
identity: test-quality-20261119T003521-jay-flowers
tier: draft
as_of: "dewey@50e8ff8ff6df4f2334c0dd2d67b92d99b27f7c2d"
evidence:
  - "dewey:.uf/artifacts/dispatch/4812b6f9-699e-424a-8783-32211cbb9053.json"
---

When a content hash computed at write time must agree with a hash computed at read time across packages that cannot import each other (dewey: tools.StoreLearning in package tools vs reIngestLearnings in package main), pin the joined contract from both sides. Side A (writer's package test): derive the reader's hash by mirroring its body-extraction rules inline (frontmatter split + TrimSpace + sha256[:8] hex) and assert the stored page hash equals it. Side B (reader's package test): run the real writer end-to-end, then run the real reader over the written vault and assert zero observable work (count 0, zero embedder calls, identical block UUIDs). The pair catches drift in either direction — a writer-side hashing change fails side A; a reader-side body-extraction change (e.g., dropping a TrimSpace) fails side B — without needing an import edge. The inline mirror on side A fails loud (spurious failure) on reader drift; it can never silently pass, which is the safe failure direction for a mirrored derivation.
