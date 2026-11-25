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

## 1. Detection Workflow

- [x] 1.1 Add `.github/workflows/learnings-only.yml` (`workflow_call`) with a `changes` job and a `code` output: `true` for non-PR events, empty file lists and PRs over 3000 files; otherwise `true` only when some `filename` or `previous_filename` is outside `.uf/dewey/learnings/`.

## 2. Gate the Callers

- [x] 2.1 [P] `ci.yml`: narrow `push` to `main`; add a `changes` job calling the reusable workflow with `pull-requests: read`; give `build-and-test` `needs: changes` and `if: needs.changes.outputs.code == 'true'`.
- [x] 2.2 [P] `mega-linter.yml`: add the same `changes` job and gate the `megalinter` job on it.

## 3. Verification

- [x] 3.1 Run `actionlint` on the three workflow files.
- [x] 3.2 Exercise the detection script locally with a stubbed `gh`: push, learnings only, mixed, rename from code, empty list, over 3000 files, look-alike path (`.uf/dewey/learnings.md`).
- [x] 3.3 Update the AGENTS.md CI/CD section.
- [ ] 3.4 After merge, confirm on the next learnings PR that `build-and-test` reports as skipped/passing and the PR merges without a full CI run.
