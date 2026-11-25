## Context

`build-and-test` (job key in `ci.yml`, no `name:`) is the only required status check on `main`. MegaLinter also runs on PRs but is not required. Release preflight checks both on `main` HEAD, which comes from `push` runs.

## Goals / Non-Goals

### Goals

- Learnings-only PRs show every required check passing within a couple of minutes, without building, testing or linting.
- Any other PR, including one mixing learnings with other files, runs CI exactly as before.
- `push` runs on `main` unchanged.

### Non-Goals

- Changing branch protection or introducing an aggregate `ci-ok` check. `build-and-test` is a plain, non-matrix job in the same workflow, so a job-level `if:` is enough.
- Skipping CI for other documentation-only changes.

## Decisions

### D1: Job-level `if:`, not `paths-ignore`

A workflow filtered out by `paths-ignore` never starts, so a required check it provides never reports and the PR waits on "Expected — Waiting for status to be reported". A job skipped by its `if:` reports its check as skipped, which satisfies a required check of the same name.

### D2: File list from the PR files API

`gh api --paginate repos/{repo}/pulls/{n}/files` needs no checkout or history, so the job finishes in seconds. It lists both `filename` and `previous_filename`, so moving a source file into the learnings directory still counts as a code change. The API lists at most 3000 files; if the event's `changed_files` exceeds that, the job runs full CI. An empty list also runs full CI. The calling job grants `pull-requests: read`.

### D3: One reusable workflow for both callers

`ci.yml` and `mega-linter.yml` call `./.github/workflows/learnings-only.yml`, so the rule lives in one place. Its check appears as `changes / changes`, which is not required.

### D4: Narrow `ci.yml` push to `main`

Every PR branch already gets CI from `pull_request`. Running on `push` to every branch doubled each PR's runs. Branches without a PR no longer get CI; that is accepted.

## Risks / Trade-offs

- If `build-and-test` ever becomes a matrix job or moves into a reusable workflow, a skipped job would no longer report under its required name. A comment on the job records this; the fix then is an aggregate required job.
