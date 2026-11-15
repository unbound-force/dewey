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
  pull-requests: write
  contents: read
```

Steps:

1. `gh issue list --label dewey-distill --state open --json number` → if any, `gh issue comment N --body "<!-- dewey-distill-trigger --> /dewey-distill-auto #N"`; else `gh issue create --label dewey-distill --assignee @me --title "Weekly dewey-distill" --body "Dispatch: /dewey-distill-auto"`.
2. Dedup invariant: at most one `open` labeled `dewey-distill`.

## gh-hud Dispatch (external)

`gh-hud` observing the issue dispatches `/dewey-distill-auto #{num}` as `autonomy` agent (jflowers/gh-hud#28). Contract Dewey exposes: label `dewey-distill`, assignee, body marker.

## Adopting Repo Setup

1. Copy `.github/workflows/dewey-distill.yml.example` → `.github/workflows/dewey-distill.yml`.
2. Ensure label `dewey-distill` + `human-gate` exist.
3. Ensure CODEOWNERS + ruleset 26764121 per #194.
4. Optional: add `.uf/dewey/distill-policy.yaml`.

## Tests

- Second trigger → comment, not duplicate issue.
- Fixture 4-week schedule → invariant holds.
- Template exists at documented path.
