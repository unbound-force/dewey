<!--
  [P] marks tasks eligible for parallel execution.
  Add [P] when a task: (a) touches different files from
  other [P] tasks in the group, (b) has no dependency
  on prior tasks in the group, (c) can safely execute
  without ordering constraints.
  Do NOT add [P] when tasks modify the same file —
  parallel workers will cause merge conflicts.
  Tasks without [P] run sequentially first, then [P]
  tasks run in parallel.
-->

## 1. Rewrite Release Workflow

All tasks modify `.github/workflows/release.yml` — no parallel execution.

- [x] 1.1 Replace `on: push: tags` trigger with `workflow_dispatch` trigger accepting a required `tag` string input. Add `concurrency` block (`release-${{ github.ref }}`, `cancel-in-progress: false`). Change top-level `permissions` to `permissions: {}`.
- [x] 1.2 Add `preflight` job with per-job `permissions: { contents: write, checks: read }` and `timeout-minutes: 10`. Add `RELEASE_TAG` env var from `${{ inputs.tag }}`. Include `actions/checkout` with `fetch-depth: 0` using a pinned SHA matching the existing workflow convention (e.g., `actions/checkout@<sha> # v4.2.2`).
- [x] 1.3 Add branch validation step: verify `github.ref == 'refs/heads/main'` and reject releases triggered from non-main branches with a clear error message.
- [x] 1.4 Add tag format validation step: reject tags not matching `^v[0-9]+\.[0-9]+\.[0-9]+$`. This implicitly rejects empty strings, whitespace, pre-release suffixes (`v1.2.3-rc1`), build metadata (`v1.2.3+build.1`), and non-numeric segments.
- [x] 1.5 Add tag uniqueness check step: verify tag does not exist on remote via `git ls-remote --tags origin`. If the tag exists, check whether it points to the dispatched commit (`github.sha`): if same commit, pass (idempotent re-run); if different commit, fail with error.
- [x] 1.6 Add semver ordering step: verify new tag is strictly greater than latest existing tag using `sort -V`. "Latest" means highest version by semver ordering. Handle first-release case (no existing tags). Add comment: `# sort -V is a GNU coreutils extension, available on ubuntu-latest runners`.
- [x] 1.7 Add CI status verification step: query GitHub Checks API (`gh api repos/${GH_REPO}/commits/${HEAD_SHA}/check-runs`) for `build-and-test` and `MegaLinter` checks on the dispatched commit (`github.sha`). Both MUST have `conclusion: "success"`. Check ALL required checks and report ALL failures (not fail-fast). Include descriptive error messages distinguishing "check not found" from "check found but not successful". Add comment: `# Check names must match job IDs/names in ci.yml and mega-linter.yml`.
- [x] 1.8 Add unreleased commits check: verify at least one commit exists between latest tag and the dispatched commit. Handle first-release case (no existing tags — all commits are unreleased).
- [x] 1.9 Add tag creation step: create annotated tag (`git tag -a "$RELEASE_TAG" -m "$RELEASE_TAG"`) and push to origin. Skip if tag already exists and points to the dispatched commit (idempotent re-run support). Fail if tag exists but points to a different commit.
- [x] 1.10 Move signing secrets check from `release` job to `preflight` job. Wire `has_signing_secrets` output from `preflight`.
- [x] 1.11 Update `release` job: add `needs: preflight`, add per-job `permissions: { contents: write }`, update checkout to use `ref: ${{ inputs.tag }}`, add `RELEASE_TAG` env var from `${{ inputs.tag }}`. Replace all `${GITHUB_REF_NAME}` references with `$RELEASE_TAG` (including the `gh release upload` command for the cask). Forward `has_signing_secrets` output from preflight so `sign-macos` can access it.
- [x] 1.12 Update `sign-macos` job: add per-job `permissions: { contents: write }`. Add `RELEASE_TAG` env var from `${{ inputs.tag }}`. Replace ALL `${GITHUB_REF_NAME}` references with `$RELEASE_TAG` (7 locations in the current workflow: asset download, version extraction, signed asset upload, checksum download, checksum upload, cask version extraction, and cask download). Update `needs` and output references to use the preflight output path for `has_signing_secrets`. Verify no `GITHUB_REF_NAME` references remain after edits.

## 2. Verification

- [x] 2.1 Validate the complete workflow YAML is valid by running `python3 -c "import yaml; yaml.safe_load(open('.github/workflows/release.yml'))"` or equivalent YAML lint. Note: this validates YAML syntax only, not GitHub Actions schema — the real validation is the first workflow execution.
- [x] 2.2 Verify no remaining `GITHUB_REF_NAME` references exist in the workflow file (grep check).
- [x] 2.3 Verify constitution alignment: confirm the change maintains Observable Quality (preflight produces auditable step output), Composability First (no new dependencies on external tools), and does not affect Autonomous Collaboration or Testability (N/A per proposal).
- [x] 2.4 Update `AGENTS.md` CI/CD section: change the Release workflow description from "Triggered on `v*` tag push" to reflect the `workflow_dispatch` trigger, the new preflight job with its 6 validation steps, and the `gh workflow run` command for scriptable releases. Verify the updated description is accurate.
<!-- spec-review: passed -->
<!-- code-review: passed -->
