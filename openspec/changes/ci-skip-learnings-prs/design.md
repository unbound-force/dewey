## Context

`build-and-test` (job key in `ci.yml`, no `name:`) is the only required status check on `main`, set by classic branch protection (dewey is not in the org's safe-settings). MegaLinter also runs on PRs but is not required; it is the only place gitleaks runs. Release preflight checks `build-and-test` and `MegaLinter` on `main` HEAD, which come from `push` runs.

Sibling repos solved the same issue in unbound-force/gaze#373, unbound-force/replicator#185 and unbound-force/unbound-force#1202. This design takes the merge-commit diff from replicator, the symlink and `..` checks from gaze, and the tested standalone script and aggregate check from unbound-force.

## Goals / Non-Goals

### Goals

- Learnings-only PRs show every required check passing within a couple of minutes, without building, testing or linting.
- Any other PR, including one mixing learnings with other files, runs CI exactly as before.
- No way for a detection failure to turn into a passing required check.
- Learnings PRs are still scanned for secrets.
- `push` runs on `main` unchanged.

### Non-Goals

- Skipping CI for other documentation-only changes.
- Managing dewey's branch protection through safe-settings.

## Decisions

### D1: Job-level `if:`, not `paths-ignore`

A workflow filtered out by `paths-ignore` never starts, so a required check it provides never reports and the PR waits on "Expected — Waiting for status to be reported". A job skipped by its `if:` reports as skipped, which counts as passing.

### D2: `CI OK` aggregate as the required check

If `changes` fails, GitHub skips every job that needs it, and a skipped `build-and-test` would satisfy the required check: the PR would go green with no CI. `CI OK` runs under `if: always()` and passes only when `changes` succeeded and the job it chose passed (`build-and-test` for `code=true`, `gitleaks` for `code=false`). It also keeps later changes to the job list out of branch protection.

### D3: Diff the PR merge commit

On `pull_request`, checkout gives `refs/pull/N/merge`, the commit CI tests. With two commits of history, `git diff --raw --no-renames HEAD^1 HEAD` is exactly what merging changes, even when the branch is behind `main`, and there is no file-count limit (the files API stops at 3000). The job first checks that `HEAD^2` is the PR head and fails otherwise.

### D4: Standalone, tested detector that fails closed

`scripts/detect-learnings-only.sh` reads the raw diff and prints `false` only when every line is a regular file (mode 100644, 100755 or deletion) under `.uf/dewey/learnings/` with no `.` or `..` segments. Symlinks, submodules, quoted paths, unparsable lines and empty input all print `true`. `--no-renames` lists both sides of a rename. Its test runs in the `changes` job, so a broken detector fails CI rather than skipping it.

### D5: gitleaks job for learnings PRs

Skipping MegaLinter skips its gitleaks run, and learnings are free text where a pasted token could land. The `gitleaks` job downloads gitleaks 8.30.1 (pinned by SHA-256) and scans `HEAD^1..HEAD^2` with the repo's `.gitleaks.toml`. It runs only when `code == 'false'`, since MegaLinter covers every other PR.

### D6: One reusable workflow for both callers

`ci.yml` and `mega-linter.yml` call `./.github/workflows/learnings-only.yml`, so the rule lives in one place.

### D7: Narrow `ci.yml` push to `main`

Every PR branch already gets CI from `pull_request`. Running on `push` to every branch doubled each PR's runs. Branches without a PR no longer get CI; that is accepted.

## Rollout

1. Merge this change. `build-and-test` stays required meanwhile, and its skip satisfies it.
2. Switch the required check from `build-and-test` to `CI OK` (repo admin):
   `gh api -X PATCH repos/unbound-force/dewey/branches/main/protection/required_status_checks --input -` with `{"strict": false, "checks": [{"context": "CI OK", "app_id": 15368}]}`.

## Risks / Trade-offs

- Until step 2, a failed `changes` job still skips `build-and-test` and passes the old required check; `CI OK` shows red but isn't required yet.
- The `gitleaks` job downloads a binary from GitHub releases; the SHA-256 pin rejects anything else.
