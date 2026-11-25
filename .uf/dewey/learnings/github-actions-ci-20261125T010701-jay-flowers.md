---
tag: github-actions-ci
author: jay-flowers
category: gotcha
created_at: 2026-11-25T01:07:01Z
identity: github-actions-ci-20261125T010701-jay-flowers
tier: draft
as_of: "dewey@e554c22817dc08b7d20ddeadc0e9c55fd79ba44d"
---

A GitHub Actions required status check gated behind a non-required job (`needs: detect` + `if: needs.detect.outputs.code == 'true'`) is fail-open: if the detect job fails — an unhandled `gh api`/network/rate-limit error under `set -euo pipefail`, or SIGPIPE from `printf | head` under `pipefail` — the required job is skipped, and GitHub reports a skipped check as passing, letting a code-changing PR merge without CI. Gate required jobs with `if: needs.detect.result != 'success' || needs.detect.outputs.code == 'true'` so a detection failure runs full CI, and make detection scripts fail-safe by catching command failures (fall back to `code=true`) rather than exiting non-zero.
