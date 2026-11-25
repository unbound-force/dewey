## Why

Learnings PRs (`chore(dewey): store learnings from …`, branches `chore/dewey-learnings-*`) only add Markdown files under `.uf/dewey/learnings/`. Nothing in them can break a build, a test or a lint, yet each one runs the full CI on every push and every update from `main`, and `ci.yml` runs twice per PR because its `push` trigger matches every branch. Where branches must be up to date, the merge train lands them one at a time behind full CI runs.

Reference: [unbound-force/dewey#251](https://github.com/unbound-force/dewey/issues/251)

## What Changes

- New `scripts/detect-learnings-only.sh` (with `scripts/detect-learnings-only_test.sh`): reads `git diff --raw --no-renames` and prints `false` only when every change is a regular file under `.uf/dewey/learnings/`.
- New reusable workflow `.github/workflows/learnings-only.yml`: runs the test and the script on the PR merge commit and outputs `code`.
- `ci.yml`: `build-and-test` runs only when `code == 'true'`. A new `gitleaks` job scans learnings-only PRs, which no longer get MegaLinter's secret scan. A new `CI OK` job fails unless detection succeeded and the job it chose passed; it becomes the required check. The `push` trigger narrows from `'**'` to `main`.
- `mega-linter.yml`: the `megalinter` job runs only when `code == 'true'`.

## Capabilities

### New Capabilities

- `learnings-only detection`: classifies a PR's merge-commit diff, failing closed.
- `gitleaks job`: secret scan of a learnings-only PR's commits.
- `CI OK job`: aggregate required check.

### Modified Capabilities

- `build-and-test job`: skipped on learnings-only PRs.
- `megalinter job`: skipped on learnings-only PRs.
- `ci.yml push trigger`: runs on `main` only.

### Removed Capabilities

- CI on pushes to branches with no pull request.

## Impact

- CI configuration and two shell scripts, plus the AGENTS.md CI/CD section. No Go code changes.
- Branch protection: after merge, the required status check on `main` changes from `build-and-test` to `CI OK`. Until then `build-and-test` stays required and the skip still satisfies it.
- `push` runs on `main` are unchanged, so the release preflight (which reads `build-and-test` and `MegaLinter` on `main`) is unaffected.

## Constitution Alignment

Assessed against the Dewey project constitution (v1.4.0).

### I. Composability First

**Assessment**: PASS. No new dependencies for the binary. The CI-only gitleaks download is pinned by version and SHA-256.

### II. Autonomous Collaboration

**Assessment**: N/A. No artifact or hero interfaces change.

### III. Observable Quality

**Assessment**: PASS. Code PRs run the full gate as before. Learnings PRs still get a secret scan, and the detector logs why it chose a full or skipped run. `CI OK` makes a failed or missing detection a failed check instead of a silent skip.

### IV. Testability

**Assessment**: PASS. The detector is a standalone script with a 22-case test that CI runs on every PR. The `CI OK` decision and the merge-commit detection were exercised locally; the workflows pass `actionlint`.
