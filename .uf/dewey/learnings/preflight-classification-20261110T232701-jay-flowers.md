---
tag: preflight-classification
author: jay-flowers
category: gotcha
created_at: 2026-11-10T23:27:01Z
identity: preflight-classification-20261110T232701-jay-flowers
tier: draft
---

On this machine, local golangci-lint 2.14.0 (go1.26.5) reports 7 staticcheck findings in dewey that CI does not: QF1001 De Morgan in artifacts/contract_test.go and artifacts/enumerate_stash.go, QF1012 WriteString(fmt.Sprintf) in tools/learning.go, and SA1019 deprecated Tags in tools/learning_test.go. Do not classify these as branch-caused. Prove the classification by running the same local binary in a detached worktree at the base SHA: if the identical findings reproduce there, and the MegaLinter check at that SHA concluded success (MegaLinter pins an older golangci-lint), the failure is environmental tool-version drift and must be recorded in the pre-flight inventory rather than entering the fix loop.
