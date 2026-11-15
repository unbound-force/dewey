# Quickstart: Weekly Unattended Distill

## Enable weekly distill

1. Scaffold and copy the template (a `dewey init` in the vault writes the inert `.example` — never overwriting an existing file — so adopting repos need not copy from the Dewey repository):

```bash
dewey init   # scaffolds .github/workflows/dewey-distill.yml.example if absent
cp .github/workflows/dewey-distill.yml.example .github/workflows/dewey-distill.yml
# ensure labels exist
gh label create dewey-distill --color 0E8A16 --description "Unattended weekly distill" 2>/dev/null || true
gh label create human-gate --color D93F0B --description "Requires code-owner review" 2>/dev/null || true
```

2. Prerequisites (per #194): `CODEOWNERS` owns `.uf/dewey/compiled/**`, ruleset 26764121 requires code-owner approval. Without it PRs open but bot could approve (non-governed).

3. Supervisor prerequisite (hard, per jflowers/opencode-supervisor#51): unattended mode requires `opencode-supervisor` support for a `command: dewey-distill-auto` profile in global config (denying `question` and `validated` writes). Until that ships, unattended mode is **not enabled** — the workflow still fires, but the run exits as a no-op, records a run artifact with `no_op_reason: supervisor command profile unavailable`, and posts the notice comment `supervisor command profile unavailable, unattended distill skipped`. No distillation happens and nothing is lost; re-enable by installing the supervisor profile.

4. Install dispatch (optional): `gh-hud` watches `dewey-distill` issues and dispatches `/dewey-distill-auto` as `autonomy`. Dispatch keys off the `dewey-distill` label — the workflow's created issue carries no assignee (the `GITHUB_TOKEN` installation identity cannot resolve `@me`).

5. Optional policy:

```yaml
# .uf/dewey/distill-policy.yaml
exclude_tags: ["scratch-*"]
exclude_stores: ["team-knowledge-draft"]
```

6. Manual trigger: `gh workflow run dewey-distill.yml` or push dispatch comment.

## Backlog bootstrap (one-time, ~1,729 learnings)

Run in a watched TUI pane once: `/dewey-distill-auto --backlog-from 2026-10-25 --dry-run` to preview themes (`--backlog-from` bounds the scope to learnings created on or before the date via the `scope` tool's `backlog_from` input; `--dry-run` passes `dry_run: true` to `rolling_pr`, which returns `would_create`/`would_update` per theme without mutating anything or recording a run artifact — the same clean-tree precondition still applies), then without `--dry-run` to open rolling `dewey-distill/<theme>` PRs. Weekly schedule handles steady state after.

Note: a real run takes over the vault's working tree (per-theme branch checkouts, article writes, rebase, push) — commit or stash tracked changes first (the run refuses on a dirty tracked tree) and do not edit the vault while it runs.

## Verify

- Scope determinism: `go test -run Scope -count=1 -race ./...` — 10 learnings / 2 cited → 8 in-scope.
- Dedup: run workflow twice → one open issue + comment.
- Never-question / never-validated: log grep in `tools/*_test.go` fixtures.
- Merge one `dewey-distill/<theme>` PR at `validated` → the merged, code-owner-approved PR is the vouch recorded in git; #203 blob-equality derivation yields `validated` (verified in tests through the seam the indexing routes use — a blob resolver built by `gitref.NewPageBlobResolver(ctx, opts, []string{".uf/dewey/compiled"})` feeding `vault.DeriveEffectiveTier` — distill articles live under `.uf/dewey/compiled/`, which the `dewey doctor`/`status` vouch display does not yet surface for the workspace layout; see spec US6 known limitation and unbound-force/dewey#210).

## What to watch

- Open theme PRs under `dewey-distill/*` labeled `human-gate`.
- PR bodies: per-article diff, newly cited ids, supersessions, lint drift.
- Conflict label `needs-human-resolution` → resolve manually.

## Revert

Remove/disable `.github/workflows/dewey-distill.yml`; interactive `/dewey-distill` unaffected.
