---
tag: preflight-causality
author: jay-flowers
category: gotcha
created_at: 2026-11-16T19:46:47Z
identity: preflight-causality-20261116T194647-jay-flowers
tier: draft
as_of: "dewey@c065dd5b2329438999360970de26e597918508ef"
evidence:
  - "dewey:cli.go"
  - "dewey:.golangci.yml"
  - "dewey:.github/workflows/ci.yml"
  - "dewey:.github/workflows/mega-linter.yml"
---

On the dewey repository, the local mise-installed golangci-lint (v2.15.0) flags seven staticcheck findings (QF1001 in artifacts/contract_test.go:28 and artifacts/enumerate_stash.go:137, QF1012 in tools/learning.go:416-419, SA1019 in tools/learning_test.go:410/465) that CI never sees, because MegaLinter bundles its own pinned golangci-lint version and stays green on main. During the /uf.unleash code-review pre-flight on opsx/scaffold-bash-terminal-denies (2026-11-16), the Tier 1 CI API baseline (MegaLinter: success on main) would have misclassified these local failures as branch-caused; the correct classification came from re-running the same local binary in a detached worktree at the merge-base (Tier 2), which reproduced the identical seven issues and proved them pre-existing — four of the seven sit in files the branch never touched. When local and CI linter versions diverge, the worktree baseline with the same binary, not the CI check conclusion, is the authoritative causality comparison.
