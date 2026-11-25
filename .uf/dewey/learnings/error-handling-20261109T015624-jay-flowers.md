---
tag: error-handling
author: jay-flowers
category: gotcha
created_at: 2026-11-09T01:56:24Z
identity: error-handling-20261109T015624-jay-flowers
tier: draft
---

Dewey tools package: decoding learning-page properties — untrusted JSON that arrives via re-ingested frontmatter (the #156 trust boundary) — requires a fail-closed guard: error on invalid JSON, with empty string and literal JSON null both decoding to a non-nil empty map before any write to the map. As of PR #159 (lint-anchor-drift) this guard exists in three unsynchronized places: decodeLearningProperties (tools/lint_anchors.go — named, documented, tested by TestMarkStaleUnverified_FailsClosed) and inline copies in resolveSupersededTarget and linkSuperseded (tools/learning_temporal.go; the read-only site omits nil-rehydration only because nil-map reads are safe in Go). Until a follow-up change converges all three on the named decoder, any hardening applied to one copy must be replicated at the other two, or untrusted-properties handling diverges silently across the store/markdown dual-write paths.
