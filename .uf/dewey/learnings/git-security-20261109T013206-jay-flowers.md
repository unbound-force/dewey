---
tag: git-security
author: jay-flowers
category: gotcha
created_at: 2026-11-09T01:32:06Z
identity: git-security-20261109T013206-jay-flowers
tier: draft
---

Shelling out to `git diff` inside an untrusted repository can execute repository-configured commands: porcelain `git diff` honors `diff.external` (config), external diff drivers activated by versioned `.gitattributes` (`diff=<driver>` + `diff.<driver>.command` from config), and `diff.<driver>.textconv` filters. When a tool runs git against a vault treated as untrusted (dewey lint anchor checks, tools/lint_anchors.go PathChanged, PR #159 feedback item 8), pass `--no-ext-diff --no-textconv` so the comparison is raw and machine-independent. Do not rely on `--quiet` skipping driver invocation — quick mode bypassing patch generation is a git implementation detail, not a documented contract. A configured driver also corrupts the result itself: one that exits 0 masks drift, and a textconv filter can normalize away byte-level change. `rev-parse` and `cat-file -e` never reach diff machinery, so only diff invocations need the flags.
