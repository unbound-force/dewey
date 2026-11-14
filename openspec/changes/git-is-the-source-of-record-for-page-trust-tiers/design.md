## Context

Today `promote` writes `tier = 'validated'` into the local SQLite `pages` table and stops there. The promotion never reaches git, never reaches a teammate, and never survives a disposable workspace. Two other paths silently discard trust: `main.go:1087` (from #145) clamps any learning-claimed tier to `draft` on re-ingest, and `store/migrate.go:112` (v1→v2, current `schemaVersion = 3`) resets compiled pages to `draft` on migration. Measured on `unbound-force/main` 2026-11-13, 38 of 39 compiled articles say `tier: validated` in git and ingest as `draft` everywhere. A second side effect (#204) is that regenerating an article changes its blob so it should read `draft` until re-merged.

The proposal replaces the promotion-manifest sketch from #193/#194 with a protected-branch primitive: a merged, human-approved PR is the vouch. No shared service.

## Goals / Non-Goals

### Goals
- Git blob equality is the trust authority for compiled/curated pages: `tier: validated` in file + blob equals `<remote>/<default-branch>` fetched ref means `validated` (GTS-FR-001).
- Raising trust only from the default branch; working-tree edits, unmerged branches, stale fetches cannot self-promote (GTS-FR-002).
- Lowering trust honored everywhere: `stale`, `superseded`, validity downgrades, frontmatter downgrades win regardless of branch (GTS-FR-003).
- `promote` becomes a file edit on a branch for PR review; DB tier is a cache of the git rule (PW-FR-001..003, GTS-FR-005).
- Learnings stay `draft` forever even on default branch (≈500/week, humans vouch for compiled articles) (IP-FR-002).
- Non-git vaults and missing-refs fall back to `draft` offline, without network fetch (GTS-FR-004, IP-FR-004).
- Migration no longer clobbers compiled pages; ingestion no longer clamps compiled pages to `draft` (IP-FR-002, IP-FR-003); fixes #204 as side effect.

### Non-Goals
- CODEOWNERS / required code-owner review on compiled/curated paths — #194 (knowledge-repo settings, not this code change).
- Consumer #193 (unattended distill) — builds on this primitive.
- Network fetch during indexing; trust derivation is offline from local fetched refs.
- Changing learning `validity`/`stale`/`superseded` semantics or the 37 inherited graphthulhu read tools.
- SQLite schema change beyond migration tweak (no new tables/columns).
- New dependencies or CGO.

## Decisions

### D1 Trust authority is blob equality, not DB

The file's `tier: validated` is a request; authority is blob equality against the fetched default-branch ref. This makes trust a versioned artifact (git object) rather than opaque local state. It satisfies Observable Quality (tier is `git ls-tree`/`cat-file` inspectable, `dewey doctor` can show fetched ref vs. file blob) and Autonomous Collaboration (teammates vouch via PR merge, not runtime coupling). Two servers with different DBs but same fetched refs derive same tiers.

### D2 Offline local-git derivation with batch fast path

Indexing MUST resolve `<remote>/<default-branch>` from local git state only — the remote-tracking ref already fetched, no network. The effective ref configuration is resolved in exactly one precedence order, highest first: per-vault `.uf/dewey/config.yaml` → global `~/.config/dewey/config.yaml` → environment variable `DEWEY_DEFAULT_BRANCH_REF` → git config (`dewey.defaultBranchRef`) → default `origin/main`; the first source providing a value wins. The environment variable value grammar is `<remote>/<branch>` (for example `origin/main`, `upstream/main`) and MUST be validated per `git check-ref-format` before use like any other leg. The precedence leg that supplied the effective ref (or that failure fell back to draft) MUST be recorded in diagnostics observable in logs and via `dewey doctor`. The order is deterministic, so two servers with the same fetched ref and the same effective configuration derive the same tiers (GTS-FR-005) and any configuration difference is explainable rather than heuristic. Batch fast path: one `git ls-tree -r <ref> -- <source-paths>` per source per index, compare N entries in memory; fallback to per-page `git hash-object`/`cat-file` when batch unavailable (non-git, shallow without ref, etc.). This keeps the performance cost bounded (one ls-tree per source, not per page) and preserves Composability First (no new service).

### D3 Downgrade-ever-honored override

After blob-equality grants `validated`, a second pass applies any lowering: `validity: stale`/`superseded`, `tier: draft` in file, etc. Lowering wins even when blob matches. This is fail-safe: stale knowledge never appears validated, manual downgrade on any branch takes effect immediately, and validity remains a safety net. It also covers regenerated-but-not-re-merged articles (blob mismatch already yields draft; downgrade path keeps explicit demotions durable).

### D4 Promote edits file, not DB

`tools/promote.go` and MCP registration change from `UPDATE pages SET tier='validated'` to frontmatter mutation (`tier: validated`) on the current branch. The DB tier becomes a cache recomputed on next index via D1/D3. This makes promotion visible as a branch diff/PR and respects the "raising only from default branch" rule (PW-FR-003): branch-only promotion indexes as `draft` until merged.

### D5 Learnings stay draft — keep existing clamp

`main.go:1087` clamp ("ignoring non-draft tier from learning file") remains for `learning/*.md` / `learnings/*.md` paths; IP-FR-002 removes it only for compiled/curated paths. Rationale: ≈500 learnings/week, per-learning review infeasible; humans vouch for compiled articles that cite learnings. Learnings retain `validity`/`stale`/`superseded` as raw material, but `tier` never escalates.

### D6 Migration is deterministic: preserve stored tier

`store/migrate.go` v1→v2→v3 migration MUST NOT modify `pages.tier` for any existing rows; the stored tier value is kept unchanged and the effective tier is re-derived only on the next index per IP-FR-001 (GTS-FR-001 blob equality + GTS-FR-003 downgrade). `schemaVersion = 3`; the previous `store/migrate.go:112` backfill that forced compiled pages to `draft` is deleted. A fresh DB built from a vault whose compiled pages are `validated` on the default branch ingests as `validated` via IP-FR-001; an upgrade from v2 with a row at `validated` leaves `pages.tier` unchanged and the next index reflects IP-FR-001. Learning migration behavior is unchanged.

### D7 Deterministic ref resolution

Ref resolution is deterministic for a given local ref state: configured remote + branch, local `origin/main` SHA, file blob SHA. Before any git invocation, the configured `<remote>/<default-branch>` value is validated per `git check-ref-format` rules — bounded length, no leading `-`, no whitespace or control characters, no `..` — and git plumbing is always invoked via argv arrays with `--` separators, never through a shell string; a malformed value degrades to the draft fallback (GTS-FR-004) with a diagnostic rather than reaching git. Diagnostics surface missing ref (GTS-FR-004 fallback) via `dewey doctor`. No heuristic branching on network availability.

## Risks / Trade-offs

- **Missing fetched ref fallback is draft — intentional but noisy.** A teammate who hasn't fetched `origin/main` will see `draft` until they fetch. Mitigation: `dewey doctor` notes missing ref; `dewey status` shows fetched ref SHA.
- **Shallow / detached workspaces.** Shallow clones without default-branch ref or detached HEAD still fall back to `draft`. Acceptable: pure SQLite vaults continue to work with observable draft fallback (proposal compat note); no Tier-1 contract change beyond tier value correctness.
- **Performance regression if batch unavailable.** Per-page `hash-object` fallback is slower but only triggers when `ls-tree -r` cannot run; N is bounded by compiled-curated page count (tens, not thousands).
- **File edit race in promote.** `promote` mutates frontmatter on disk; concurrent edits to same file could conflict. Acceptable: promotion is human-gated PR path, and git merge conflict surfaces naturally (same file edited).
- **Regeneration churn.** Every regeneration without re-merge reads as `draft` (fix for #204) — intended: prevents stale validated articles masquerading as current.
- **Test complexity with fixture repos.** Tier tests need temporary git dirs with fetched refs. Mitigation: `t.TempDir()` + `exec.Command("git", ...)` fixtures, isolated per test, `-race -count=1`.

<!-- scaffolded by uf vdev -->
