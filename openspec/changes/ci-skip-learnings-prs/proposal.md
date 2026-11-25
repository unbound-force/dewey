## Why

Learnings PRs (`chore(dewey): store learnings from …`, branches `chore/dewey-learnings-*`) only add Markdown files under `.uf/dewey/learnings/`. Nothing in them can break a build, a test or a lint, yet each one runs the full CI on every push and every update from `main`, and `ci.yml` runs twice per PR because its `push` trigger matches every branch. Where branches must be up to date, the merge train lands them one at a time behind full CI runs.

Reference: [unbound-force/dewey#251](https://github.com/unbound-force/dewey/issues/251)

## What Changes

- New reusable workflow `.github/workflows/learnings-only.yml` that outputs `code: 'true'` unless the run is a pull request whose changed files are all under `.uf/dewey/learnings/`.
- `ci.yml`: `build-and-test` (the required check) gains `needs: changes` and `if: needs.changes.outputs.code == 'true'`. The `push` trigger narrows from `'**'` to `main`.
- `mega-linter.yml`: the `megalinter` job gets the same gate.

## Capabilities

### New Capabilities

- `learnings-only detection`: lists a PR's changed files (both sides of renames) and decides whether the expensive jobs must run.

### Modified Capabilities

- `build-and-test job`: skipped, and reported as passing, on learnings-only PRs.
- `megalinter job`: skipped on learnings-only PRs.
- `ci.yml push trigger`: runs on `main` only.

### Removed Capabilities

- CI on pushes to branches with no pull request.

## Impact

- CI configuration only: three workflow files plus the AGENTS.md CI/CD section. No Go code changes.
- Branch protection is unchanged: `build-and-test` stays the only required check, and a job skipped by its `if:` satisfies it.
- `push` runs on `main` are unchanged, so the release preflight (which reads `build-and-test` and `MegaLinter` on `main`) is unaffected.

## Constitution Alignment

Assessed against the Dewey project constitution (v1.4.0).

### I. Composability First

**Assessment**: PASS. No new dependencies for the binary; the detection step uses only `gh` and `grep`, preinstalled on the runner.

### II. Autonomous Collaboration

**Assessment**: N/A. No artifact or hero interfaces change.

### III. Observable Quality

**Assessment**: PASS. Every PR that changes code, tests or configuration still runs the full gate. The detection step logs why it decided to run or skip.

### IV. Testability

**Assessment**: PASS. The detection script was exercised locally with a stubbed `gh` (push, learnings only, mixed, renamed from code, empty, over the API limit, look-alike path), and the workflows pass `actionlint`. The first learnings PR after merge is the integration test.
