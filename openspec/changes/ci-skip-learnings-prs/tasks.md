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

## 1. Detection

- [x] 1.1 Add `scripts/detect-learnings-only.sh`: reads `git diff --raw --no-renames`, prints `false` only for regular `.md` files under `.uf/dewey/learnings/` with no `.`/`..` segments; `true` for anything else, including empty input.
- [x] 1.2 Add `scripts/detect-learnings-only_test.sh` covering learnings added/edited/deleted, non-`.md` files in the directory, empty input, code files, mixed, renames from code, look-alike paths, `.`/`..`, symlinks, submodules, quoted paths and malformed lines.
- [x] 1.3 Add `.github/workflows/learnings-only.yml` (`workflow_call`): on PRs, check out the merge commit (`fetch-depth: 2`), run the test, verify `HEAD^2` is the PR head, run the detector on `HEAD^1..HEAD`; on other events output `code=true`.

## 2. Gate the Callers

- [x] 2.1 `ci.yml`: narrow `push` to `main`; add the `changes` job; gate `build-and-test` on `code == 'true'`; add a `gitleaks` job (pinned 8.30.1, SHA-256 checked) for `code == 'false'`; add `CI OK` (`if: always()`) requiring `changes` success and the chosen job's success.
- [x] 2.2 `mega-linter.yml`: add the `changes` job and gate `megalinter` on it.

## 3. Verification

- [x] 3.1 Run `actionlint` and `shellcheck`.
- [x] 3.2 Run `bash scripts/detect-learnings-only_test.sh` (26 cases).
- [x] 3.3 Simulate PR merge commits locally (learnings-only branch behind `main`, and mixed) and confirm the detector output.
- [x] 3.4 Exercise the `CI OK` step with every combination of `changes`, `gitleaks` and `build-and-test` results.
- [x] 3.5 Confirm the gitleaks command passes on a real learnings commit and fails on a planted token.
- [x] 3.6 Update the AGENTS.md CI/CD section.

## 4. Rollout

- [ ] 4.1 After merge, switch the required status check on `main` from `build-and-test` to `CI OK`.
- [ ] 4.2 On the next learnings PR, confirm `build-and-test` and `MegaLinter` are skipped, `gitleaks` and `CI OK` pass, and the PR merges without a full CI run.
