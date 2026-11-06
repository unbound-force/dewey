---
tag: test-architecture
author: jay-flowers
category: pattern
created_at: 2026-11-05T16:45:11Z
identity: test-architecture-20261105T164511-jay-flowers
tier: draft
---

Cross-package serialized contracts on a durability path need a composed round-trip test, and when the reader lives unexported in package main the test must live in the reader's package driving the writer. In dewey PR #156 (learning temporal validity), tools/learning_temporal.go writes frontmatter/properties with string-literal keys while main.go's learningFrontmatter reads the same keys through yaml struct tags — Go struct tags are compile-time string literals, so the tag side cannot share constants with the writer and deduplication cannot pin the seam. Per-side tests (writer-output test in tools, hand-written fixture test in main_test.go) both stay green when one side renames a key and updates its own test; drift then silently drops fields during re-ingestion, the exact recovery path that lets learnings survive graph.db deletion. The pin is one test in main_test.go that calls tools.NewLearning(...).StoreLearning (main can import tools; tools cannot import main — no cycle), closes the store, re-ingests via reIngestLearnings into a fresh store, and asserts every field — including the superseded_by back-link written by setFrontmatterKeys — equals the original input, with element-value assertions on list fields (evidence), not length checks. Extends the PR 145 seam-test pattern (learning/test-architecture-20261104T183627-jay-flowers) to recovery paths where fixture tests bypass the writer entirely.
