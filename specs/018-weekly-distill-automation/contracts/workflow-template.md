# Contract: Template Workflow

## File

`.github/workflows/dewey-distill.yml.example` (example template; copy to `dewey-distill.yml` to enable)

## Triggers

```yaml
on:
  schedule:
    - cron: "0 9 * * 1"  # Mon 09:00 UTC, editable
  workflow_dispatch:
```

## Job: distill-trigger

```yaml
permissions:
  issues: write
  contents: read
```

Least privilege: the job only creates/comments issues and reads the repository — no PR write is granted (the PRs are opened by the distiller pipeline server-side through the `rolling_pr` tool, not by this workflow).

Steps:

1. `gh issue list --label dewey-distill --state open --json number` → if any, `gh issue comment N --body "<!-- dewey-distill-trigger --> /dewey-distill-auto #N"`; else `gh issue create --label dewey-distill --title "Weekly dewey-distill" --body "Dispatch: /dewey-distill-auto"`. The created issue carries no assignee: the workflow runs as the `GITHUB_TOKEN` installation identity, which cannot resolve `@me` (a `--assignee @me` flag fails the step under `set -eu`), and gh-hud dispatch keys off the `dewey-distill` label, not the assignee.
2. Dedup invariant: at most one `open` labeled `dewey-distill`.

## gh-hud Dispatch (external)

`gh-hud` observing the issue dispatches `/dewey-distill-auto #{num}` as `autonomy` agent (jflowers/gh-hud#28). Contract Dewey exposes: label `dewey-distill`, body marker (no assignee — dispatch keys off the label).

## Adopting Repo Setup

1. Run `dewey init` — it scaffolds `.github/workflows/dewey-distill.yml.example` (inert; never overwrites an existing file) — then copy it to `.github/workflows/dewey-distill.yml` to enable the schedule.
2. Ensure label `dewey-distill` + `human-gate` exist.
3. Ensure CODEOWNERS + ruleset 26764121 per #194.
4. Optional: add `.uf/dewey/distill-policy.yaml`.

## Tests

- Second trigger → comment, not duplicate issue.
- Fixture 4-week schedule → invariant holds.
- Template exists at documented path.
