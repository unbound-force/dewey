---
tag: learning-properties
author: jay-flowers
category: context
created_at: 2026-11-09T01:56:29Z
identity: learning-properties-20261109T015629-jay-flowers
tier: draft
---

PR #159 (lint-anchor-drift) left learning-properties JSON decoding with three near-identical null/empty guards: decodeLearningProperties in tools/lint_anchors.go (L289) and the inline guards in resolveSupersededTarget (L195-200) and linkSuperseded (L215-223) in tools/learning_temporal.go. Maintainer review (PR #159, comment on lint_anchors.go:289) deferred convergence to a follow-up change to keep the frozen OpenSpec scope intact. The copies differ subtly in error contract (error-return vs warning-string vs nil-tolerant read), so a shared decoder must adopt the strictest contract: empty string and literal JSON null decode to an empty map, invalid JSON is an error. Future code decoding learning page properties should extract or reuse that single decoder rather than adding a fourth copy; the natural home is tools/learning_temporal.go beside the already-shared helpers (ValidLearningIdentity, setFrontmatterKeys, gitOutput). The follow-up can bundle with the dual-write-path consolidation from PR #159 feedback items 3/7, which share the same root cause.
