---
tag: ci-path-gating
author: jay-flowers
category: gotcha
created_at: 2026-11-24T18:10:26Z
identity: ci-path-gating-20261124T181026-jay-flowers
tier: draft
as_of: "dewey@beae8a6b20b79a0d95b9a39662d536715396a1d5"
---

Piping `git diff` into `grep` inside an `if` condition silently turns a git failure into the false branch: with or without pipefail, a failing `git diff` produces empty stdout, so `... | grep -q .` exits non-zero and the `else` runs. In a CI path-gating job that means a diff error yields `code=false` and skips the expensive jobs — a fail-open that `set -e` cannot catch because the failure is being tested. Capture the diff output as a standalone command first (`changed=$(git diff ...)` under `set -e`, or check its exit status explicitly), then branch only on a successful result. Add a test that forces a diff failure and asserts a non-zero job exit.
