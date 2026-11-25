---
tag: ci-parity
author: jay-flowers
category: gotcha
created_at: 2026-11-16T00:37:44Z
identity: ci-parity-20261116T003744-jay-flowers
tier: draft
---

In fork checkouts whose origin remote points at a personal fork with a stale main, pre-flight causality classification must baseline against merge-base(HEAD, upstream/main), not the symbolic-ref-detected origin/main. On 018-weekly-distill-automation the fork's origin/main sat at 15f7eea (predating dewey #146), so seven golangci-lint staticcheck findings in tools/learning.go, tools/learning_test.go, and artifacts/ looked branch-caused against the stale base. File-identity plus git blame at the upstream merge base eb4720c proved all four flagged files byte-identical there — conclusively pre-existing and out of the change's scope. For lint findings, file identity at the true merge base is a complete baseline: identical code cannot be branch-caused, so no worktree re-run is needed. A naive worktree baseline at the stale fork ref would have misclassified #146-era upstream code as branch-caused and wrongly pulled it into the fix loop.
