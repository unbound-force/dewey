---
tag: ci-pipeline
author: jay-flowers
category: gotcha
created_at: 2026-11-24T23:50:17Z
identity: ci-pipeline-20261124T235017-jay-flowers
tier: draft
as_of: "dewey@beae8a6b20b79a0d95b9a39662d536715396a1d5"
---

A GitHub Actions path-detection step written as `if git diff ... | grep -v PREFIX | grep -q .; then code=true; else code=false; fi` is fail-open: without `pipefail` the pipeline's status is the last grep's, so a failed `git diff` (missing base/head SHA, unreachable commit) and an empty diff both land in the else branch and report "no code changes". When the gated job is the required check, the skip satisfies branch protection and the PR merges with zero build or test. Capture the diff in an explicit `if ! changed=$(git diff ...)` branch under `set -euo pipefail`, guard the SHAs with `${VAR:?}`, and map an empty diff to "run full CI".
