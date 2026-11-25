---
tag: test-quality
author: jay-flowers
category: pattern
created_at: 2026-11-03T18:43:12Z
identity: test-quality-20261103T184312-jay-flowers
tier: draft
---

In dewey's twin OpenAI-compatible provider test files (embed/openai_compatible_test.go and llm/openai_compatible_test.go), table-driven error-path tests pin per-case error messages with a wantErr/wantWord substring column — see TestOpenAICompatibleEmbedder_OrderingErrors, which asserts strings.Contains(err.Error(), tt.wantWord) for each case. When adding or extending error-path table tests in these packages, include the substring column rather than asserting only err != nil: spec scenarios worded as "returns a descriptive error" (e.g. 017 US1-6) make the message text part of the observable contract, and go.md TC-009 treats err-only assertions as a MUST violation. PR #144 feedback item 7 caught exactly this drift between the twin files — the llm EmptyContent test omitted the column its embed counterpart had. Prefer substring matching over full-message equality to avoid over-pinning (the NonSuccessError full-message assertion is the stricter exception, not the rule).
