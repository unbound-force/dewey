---
tag: review-dispatch
author: jay-flowers
category: gotcha
created_at: 2026-11-09T00:04:16Z
identity: review-dispatch-20261109T000416-jay-flowers
tier: draft
---

Divisor review of a PR diff can execute against a checkout pinned at the PR's base SHA: files added by the diff (for example tools/lint_anchors.go in dewey PR #159) do not exist on disk, while the diff's context lines match local files exactly. Before concluding a diff-added file or symbol is missing, verify the diff's before-hunks against local files, confirm referenced helpers exist at base via grep, and treat the diff text as the authoritative review subject.
