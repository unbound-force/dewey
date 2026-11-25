---
tag: test-assertions
author: jay-flowers
category: pattern
created_at: 2026-11-03T18:43:08Z
identity: test-assertions-20261103T184308-jay-flowers
tier: draft
---

In table-driven Go error tests, assert a per-case message substring (wantErr/wantWord column with strings.Contains) rather than only err != nil — especially when the implementation deliberately returns distinct contract messages. In dewey spec 017, the wire contract makes error text contract surface ("Descriptive error; no article persisted"), go.md TC-009 [MUST] requires specific expected values in assertions, and both the embed and llm openai_compatible test suites establish the wantWord/wantErr pattern. An err != nil-only table test passes even if a refactor collapses distinct contract errors into one generic message, silently regressing the spec's "descriptive error" acceptance criterion (US1-6). Substring (not exact-match) assertions on contract-documented messages keep brittleness low while pinning the discrimination.
