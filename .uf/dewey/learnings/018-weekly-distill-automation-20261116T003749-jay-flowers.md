---
tag: 018-weekly-distill-automation
author: jay-flowers
category: context
created_at: 2026-11-16T00:37:49Z
identity: 018-weekly-distill-automation-20261116T003749-jay-flowers
tier: draft
---

The 018-weekly-distill-automation code council (correlation a96a02ec, head 72a362a, 2026-11-16) returned APPROVE WITH ADVISORIES with five divisor-sre findings recorded as advisories — the natural follow-up candidates for a post-merge hardening change: MEDIUM tools/rolling_pr.go:700, the restore closure in prepareDistillRun discards git checkout errors, so a failed restore leaves the vault silently sitting on a dewey-distill branch; MEDIUM gate.go:1137, recordSupervisorNoOp builds no-op run artifacts with schema 0 while every other producer writes 1 because distillRunRecordSchema is unexported and unreachable from package main; LOW tools/rolling_pr.go:1257, remoteBranchSHA conflates an absent remote branch with an unreachable remote (empty-SHA sentinel on any non-zero ls-remote exit); LOW tools/scope.go:870 and tools/group.go:710, per-object git subprocess reads (cat-file/show one process per object) could collapse to git cat-file --batch; LOW .opencode/commands/dewey-distill-auto.md:141, the command prose documents a prs_touched object shape and agent-authored run artifacts that the implementation flattened to branch-name strings and moved server-side.
