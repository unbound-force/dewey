<!-- originating_issue: 203 -->

## Why

Each teammate runs their own Dewey MCP server. The trust tier (`draft` / `validated` / etc.) lives today only in that server's local SQLite database. `promote` writes `tier = 'validated'` into `pages` and stops there — the promotion never reaches git, never reaches a teammate, and never survives a disposable fleet workspace. Two other paths silently discard trust: learning files that claim a tier are re-ingested as `draft` (`main.go:1087`, from #145), and the v1→v2 migration resets compiled pages to `draft` (`store/migrate.go:112`). Measured on `unbound-force/main` on 2026-11-13, 38 of 39 compiled articles say `tier: validated` in git and ingest as `draft` everywhere. Replacing the promotion-manifest approach originally sketched in #193/#194, this change makes the default branch's protection the trust boundary: a merged, human-approved PR is the vouch.

## What Changes

- **Git blob is the source of record.** A compiled or curated page is `validated` only when its file says `tier: validated` **and** its blob equals the blob at `<remote>/<default-branch>` (the fetched ref). Every server derives this locally from git, offline, with one `git cat-file`/`ls-tree` per page at ingest or one `ls-tree -r` per source per index run. No shared service.
- **Raising trust comes only from the default branch.** A working-tree edit, an unmerged branch, or a stale fetch that differs from the default-branch blob reads as `draft`. Local edits cannot self-promote.
- **Lowering trust is honored everywhere.** `stale`, `superseded`, validity downgrades, and any frontmatter that lowers trust are fail-safe and honored from any file, branch, or local edit.
- **`promote` becomes a file edit.** `dewey promote` / the `promote` MCP tool sets `tier: validated` in the page's frontmatter on a branch so a PR can be opened. It no longer writes `pages.tier = 'validated'` directly. The database tier becomes a cache of the git rule.
- **Learnings stay `draft` forever.** At ~500 new learnings/week, per-learning review is infeasible. Humans vouch for compiled articles that cite learnings; learnings remain raw material. `validity`/`stale`/`superseded` may still mark them, but `tier` never escalates.
- **Non-git fallback.** When the vault is not a git repo or no default-branch ref is available, every page reads as `draft` — current behavior.
- **Migration and ingestion cleanup.** Remove or invert the compiled-page `draft` backfill in `store/migrate.go`; remove the "ignoring non-draft tier" special case for compiled/curated pages (keep it for learnings per rule above).
- **Enforcement split.** CODEOWNERS + required code-owner review on compiled/curated page paths is #194 (knowledge-repo settings, not this code change). Consumer #193 (unattended distill) builds on this primitive. This change also fixes #204 as a side effect: a regenerated article's blob no longer matches the merged one, so it reads `draft` until re-merged.

## Capabilities

### New Capabilities
- `git-trust-source`: default-branch blob equality as the trust-tier authority for compiled/curated pages, with offline local git derivation and database-tier-as-cache semantics.

### Modified Capabilities
- `promotion-workflow`: `promote` edits frontmatter on a branch for PR-based promotion instead of writing the DB tier; branch-only promotions remain `draft` until merged.
- `ingest-pipeline`: ingestion derives `tier` from git blob comparison (with downgrade-ever-honored override) and removes compiled-page `draft` clamping; learnings remain clamped to `draft`.

### Removed Capabilities
- None — existing CLI/MCP surfaces remain; only their semantics change.

## Impact

- **Code**: `main.go` ingestion (tier clamping), `store/migrate.go` (backfill), `tools/promote.go` + MCP registration (frontmatter edit path), index/vault layer (git ref resolution and blob hashing), `dewey doctor`/`dewey search` tier display.
- **Docs**: `AGENTS.md`, `README.md`, and `openspec/specs/*` references to tier persistence and promotion.
- **Website sync**: per AGENTS.md's Website Documentation Sync rule, this user-facing behavior change requires a GitHub issue in `unbound-force/website` (`gh issue create --repo unbound-force/website --label docs`) covering promote semantics (frontmatter file edit for PR-based promotion), tier display and git-blob-equality derivation, `dewey doctor`/`dewey status` diagnostics, and the migration note (compiled-page tiers re-derived from git after this change). Task 5.1 files it.
- **Compat**: No Tier-1/Tier-2 contract change for graphthulhu read tools beyond tier value correctness; pure SQLite DBs still work (fallback = `draft` where git unavailable — observable, intentional).
- **Performance**: One `ls-tree -r` per source per index is the intended path; per-page `cat-file` is the fallback.

## Content Candidacy

- **Blog**: the trust-model migration — database tier demoted to a cache, default-branch blob equality as the trust authority, fixing the measured 38-of-39 validated-drift and #204 — is a blog candidacy note (architectural migration with a measurable before/after).
- **Tutorial**: the new multi-step promotion workflow — `promote` on a branch → open PR → code-owner approval → merge → fetch → re-index → `validated`, with stale-fetch surprises when the fetched ref is missing — is a tutorial candidacy note (a workflow engineers must learn; unaided discovery hits the stale-fetch failure mode).

## Constitution Alignment

Assessed against the Unbound Force org constitution (dewey constitution v1.4.0, parent org v1.1.0).

### I. Autonomous Collaboration

**Assessment**: PASS

Change uses a well-defined artifact (git blob on the default branch) as the vouch and keeps collaboration artifact-based. No runtime coupling or shared service is introduced; each hero derives trust locally from the repo it already has. `promote` remains a self-describing tool that produces a branch edit for PR review rather than mutating opaque local state that teammates cannot see.

### II. Composability First

**Assessment**: PASS

Dewey stays independently installable and usable alone. Non-git vaults, single-user setups, and consumers that never use trust tiers are unaffected — they see `draft`, the current default. No new mandatory dependency; git is already the vault's backing store where present, and the implementation degrades gracefully when unavailable. Other heroes continue to consume Dewey via MCP/tools unchanged.

### III. Observable Quality

**Assessment**: PASS

Trust becomes machine-parseable and provenance-linked: the file's frontmatter plus blob equality against the protected default-branch ref, both inspectable via `git ls-tree`/`cat-file` and surfaced by `dewey doctor`/`health`. Promotion intent is visible as a branch diff/PR rather than hidden local DB state, and two servers with different DBs but the same fetched ref derive the same tier — auditable quality.

### IV. Testability

**Assessment**: PASS

Behavior is verifiable via observable side effects: ingest tier for identical-to-default-branch vs. modified working-tree vs. stale vs. learning-claiming-validated fixtures, and `promote`-edits-file-not-DB assertions. Tests run in isolation with fixture repos (temporary git dirs, fetched ref fixtures) without external services, under `-race -count=1`.

<!-- scaffolded by uf vdev -->
