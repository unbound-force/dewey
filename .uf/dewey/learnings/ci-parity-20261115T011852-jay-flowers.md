---
tag: ci-parity
author: jay-flowers
category: context
created_at: 2026-11-15T01:18:52Z
identity: ci-parity-20261115T011852-jay-flowers
tier: draft
---

As of 2026-11-14, dewey main (commit 78170ea) carries 7 pre-existing golangci-lint staticcheck findings that make a local `golangci-lint run ./...` exit 1: QF1001 (De Morgan) x2 in artifacts/contract_test.go and artifacts/enumerate_stash.go, QF1012 (fmt.Fprintf instead of WriteString+Sprintf) x3 in tools/learning.go, and SA1019 (deprecated types.StoreLearningInput.Tags field) x2 in tools/learning_test.go. The failure set is byte-identical at the merge-base, so pre-flight gates on docs-only branches should classify it pre-existing via a detached-worktree baseline (git worktree add under /tmp at the merge-base, run the tool there, diff the outputs) and treat the gate as green over it rather than entering the fix loop — the causality classification contract says pre-existing and environmental failures never enter the loop.
