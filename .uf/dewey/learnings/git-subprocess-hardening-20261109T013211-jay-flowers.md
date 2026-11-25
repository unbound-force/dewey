---
tag: git-subprocess-hardening
author: jay-flowers
category: gotcha
created_at: 2026-11-09T01:32:11Z
identity: git-subprocess-hardening-20261109T013211-jay-flowers
tier: draft
---

Shelling out to git against a directory whose content is untrusted (dewey vaults, checked-out sources) is not neutral: the porcelain `git diff` honors repository-local configuration — `diff.external` and `.gitattributes`-driven `diff.<driver>.command` / `diff.<driver>.textconv` from that repository's .git/config — which can execute arbitrary commands. `--quiet` is not a reliable bypass across git versions, and textconv behavior under `--exit-code` must not be relied on. Pass `--no-ext-diff --no-textconv` on every `git diff` over untrusted trees (or use plumbing such as `diff-tree`, which ignores `diff.external`). Precedent: dewey PR 159 lint anchor checks — PathChanged in tools/lint_anchors.go needed the flags to satisfy the change's own fail-closed-on-untrusted-input design (D5) and the spec's 'anchor checks MUST use local git only' requirement.
