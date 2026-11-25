---
tag: test-quality
author: jay-flowers
category: pattern
created_at: 2026-11-03T18:43:03Z
identity: test-quality-20261103T184303-jay-flowers
tier: draft
---

Sibling tests authored in the same PR drift apart on assertion depth: in PR #144 the embed test TestOpenAICompatibleEmbedder_OrderingErrors (embed/openai_compatible_test.go) pins per-case error substrings via a wantWord column, while the parallel synthesis test TestOpenAICompatibleSynthesizer_EmptyContent (llm/openai_compatible_test.go:117) asserts only err != nil — a go.md TC-009 [MUST] violation that surfaced only at council feedback time (feedback item 7). Pattern: when writing a table-driven error test that mirrors a sibling test's structure, add the per-case wantErr substring column at authoring time. The cost is one struct field and one strings.Contains call; the asymmetric version costs a full review-feedback cycle to catch. Substring pinning on stable contract phrases is this repo's established convention: the 401 tests pin env-var names, validation tests pin config field names, and NonSuccessError pins the status-code error format.
