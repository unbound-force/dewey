---
tag: test-quality
author: jay-flowers
category: pattern
created_at: 2026-11-05T16:45:22Z
identity: test-quality-20261105T164522-jay-flowers
tier: draft
---

A serialization contract split across two packages needs one composed round-trip test, not two per-side fixture tests. In dewey PR #156, tools/learning_temporal.go writes learning temporal fields (string-literal keys, %q-quoted frontmatter, setFrontmatterKeys inserts) while main.go's learningFrontmatter reads them (yaml tags) — both per-side tests passed while the composed store → wipe → reIngestLearnings path was never exercised. The uncaught drift mode is asymmetric addition: a field added to the writer only (the natural evolution as follow-up features extend the field set) passes both half-tests and silently drops from page properties on restore; hand-crafted read-path fixtures mimic rather than consume the producer's rendering, so producer format changes go unchecked against the real parser. Remedy: one test that writes through the real producer (tools.NewLearning + WithAsOfResolver), opens a fresh store (reIngestLearnings skips pages already present), restores through the real consumer, and asserts restored field values — including list contents and order, not lengths (go.md TC-009). PR 145's tools/curate predicate drift is the codebase precedent that duplicated mappings do drift.
