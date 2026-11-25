---
tag: pre-flight
author: jay-flowers
category: pattern
created_at: 2026-11-12T19:01:30Z
identity: pre-flight-20261112T190130-jay-flowers
tier: draft
---

Soft-gate pre-flight failure classification in a Go repo works in two cheap steps before any CI API call: (1) check whether any lint-flagged file appears in the branch diff — when golangci-lint flagged tools/learning.go, tools/learning_test.go, and artifacts/* while the branch only touched tools/learning_temporal.go and types/tools.go, file-level absence already implies pre-existing; (2) confirm with a Tier 2 worktree baseline: `git worktree add /tmp/preflight-baseline-<sha> <merge-base> --detach`, run only the failing tool there, compare issue counts, remove the worktree. Identical counts (7 staticcheck QF/SA issues at merge-base 20fa93df for dewey #150, 2026-11-12) yield the pre-existing classification, the gate passes over the failure, and the failure rides into the review as an informational finding. The SA1019 hits on the deprecated Tags field are the intentional legacy-tags backward-compat tests — they deliberately use the deprecated field, so they are expected noise, not drift.
