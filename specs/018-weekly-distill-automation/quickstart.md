# Quickstart: Weekly Unattended Distill

## Enable weekly distill

1. Copy the template:

```bash
cp .github/workflows/dewey-distill.yml.example .github/workflows/dewey-distill.yml
# ensure labels exist
gh label create dewey-distill --color 0E8A16 --description "Unattended weekly distill" 2>/dev/null || true
gh label create human-gate --color D93F0B --description "Requires code-owner review" 2>/dev/null || true
```

2. Prerequisites (per #194): `CODEOWNERS` owns `.uf/dewey/compiled/**`, ruleset 26764121 requires code-owner approval. Without it PRs open but bot could approve (non-governed).

3. Install dispatch (optional): `gh-hud` watches `dewey-distill` issues and dispatches `/dewey-distill-auto` as `autonomy`.

4. Optional policy:

```yaml
# .uf/dewey/distill-policy.yaml
exclude_tags: ["scratch-*"]
exclude_stores: ["team-knowledge-draft"]
```

5. Manual trigger: `gh workflow run dewey-distill.yml` or push dispatch comment.

## Backlog bootstrap (one-time, ~1,729 learnings)

Run in a watched TUI pane once: `/dewey-distill-auto --backlog-from 2026-10-25 --dry-run` to preview themes, then without `--dry-run` to open rolling `dewey-distill/<theme>` PRs. Weekly schedule handles steady state after.

## Verify

- Scope determinism: `go test -run Scope -count=1 -race ./...` — 10 learnings / 2 cited → 8 in-scope.
- Dedup: run workflow twice → one open issue + comment.
- Never-question / never-validated: log grep in `tools/*_test.go` fixtures.
- Merge one `dewey-distill/<theme>` PR at `validated` → the merged, code-owner-approved PR is the vouch recorded in git; #203 blob-equality derivation yields `validated` (verified in tests via `vault.DeriveEffectiveTier` with a workspace pathspec — distill articles live under `.uf/dewey/compiled/`, which the `dewey doctor`/`status` vouch display does not yet surface for the workspace layout; see spec US6 known limitation and unbound-force/dewey#210).

## What to watch

- Open theme PRs under `dewey-distill/*` labeled `human-gate`.
- PR bodies: per-article diff, newly cited ids, supersessions, lint drift.
- Conflict label `needs-human-resolution` → resolve manually.

## Revert

Remove/disable `.github/workflows/dewey-distill.yml`; interactive `/dewey-distill` unaffected.
