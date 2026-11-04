---
tag: documentation-consistency
author: jay-flowers
category: gotcha
created_at: 2026-11-04T17:31:49Z
identity: documentation-consistency-20261104T173149-jay-flowers
tier: draft
---

When documentation quotes an error string that code treats as a contract, the doc rewrite must be verified against the emitting format string, not paraphrased. In dewey PR 145, the promote-ladder documentation pass rewrote README's store-failure quote from `failed to look up page '<name>'` to `failed to resolve page '<name>'` while tools/promote.go:142 still emits the former (and its comment declares error strings the handler's contract). Detection heuristic: for every quoted literal added to docs in a diff, grep the emitting package for the exact string — a doc quote with no source match is a contract misquote.
