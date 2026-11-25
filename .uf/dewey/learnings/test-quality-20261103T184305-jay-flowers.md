---
tag: test-quality
author: jay-flowers
category: pattern
created_at: 2026-11-03T18:43:05Z
identity: test-quality-20261103T184305-jay-flowers
tier: draft
---

Dewey's parallel provider test suites (embed/ and llm/) follow a mirroring convention: table-driven error tests carry a wantErr/wantWord substring column that pins the per-case error message (precedent: TestOpenAICompatibleEmbedder_OrderingErrors in embed/openai_compatible_test.go, PR #144). Repo precedents reinforce it — openspec/changes/fix-embed-bare-string/specs/embed-array-format requires a provider test to mirror its sibling test's assertion pattern, and openspec/changes/synthesis-endpoint-env/tasks 2.1 specifies llm tests mirror embed test patterns. New provider tests must mirror the twin suite's assertion depth per go.md TC-009: asserting only err == nil leaves error-contract discrimination unpinned and diverges from the sibling suite.
