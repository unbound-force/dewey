# Capability: ingest-pipeline

## ADDED Requirements

### Requirement: IP-FR-001 Ingestion derives tier from git blob comparison with downgrade override

During indexing (vault scan, `dewey index`/`reindex`, MCP indexing path), the ingestion pipeline MUST derive the effective tier for each compiled/curated page by: (1) computing the file's current blob SHA (or content hash via `git hash-object` / `ls-tree` entry), (2) resolving `<remote>/<default-branch>` for the repo (default `origin/main` with config override), and (3) comparing to the default-branch blob for the same path; `validated` if and only if frontmatter requests `validated` and blobs match. After that, the downgrade-ever-honored override from GTS-FR-003 MUST be applied so any lowering wins.

#### Scenario: Ingest validated when blob matches
- **GIVEN** `compiled/auth.md` declares `tier: validated` and its blob equals `origin/main:compiled/auth.md`
- **WHEN** the pipeline indexes the source
- **THEN** the persisted `pages.tier` is `validated`

#### Scenario: Ingest draft when blob diverges
- **GIVEN** `compiled/auth.md` declares `tier: validated` but its working-tree blob differs from `origin/main` for that path
- **WHEN** the pipeline indexes
- **THEN** the persisted `pages.tier` is `draft`

#### Scenario: Batch ref resolution prefers ls-tree
- **GIVEN** a source with N compiled pages
- **WHEN** the pipeline indexes that source
- **THEN** it SHOULD resolve the default-branch tree once via `git ls-tree -r <remote>/<default-branch> -- <source-path>` and compare all N entries in memory, falling back to per-page `git cat-file`/`hash-object` only when batch is unavailable

### Requirement: IP-FR-002 Remove compiled-page draft clamping, keep learning clamping

The ingestion MUST NOT force compiled or curated pages to `draft` solely because their frontmatter claims a non-`draft` tier (the current `main.go:1087` `ignoring non-draft tier from learning file` path for learnings). That clamp MUST remain for learning files (`learning/*.md` / `learnings/*.md` / `store_learning` output) which MUST always ingest as `draft` regardless of frontmatter or blob match, per the "Learnings stay draft forever" rule. Curated/compiled pages MUST be governed by IP-FR-001 instead.

#### Scenario: Compiled page allowed to be validated
- **GIVEN** `compiled/auth.md` with `tier: validated` and blob matching `origin/main`
- **WHEN** ingestion runs
- **THEN** no "ignoring non-draft tier" warning is emitted and tier is `validated`

#### Scenario: Learning claiming validated stays draft
- **GIVEN** `learning/auth-20260502T143022-alice.md` with `tier: validated` in its frontmatter and blob matching `origin/main`
- **WHEN** ingestion runs
- **THEN** the effective tier is `draft` and a diagnostic MAY note learning clamp

#### Scenario: Learning volume invariant
- **GIVEN** continuous ingestion with ~500 new learnings/week in the vault
- **WHEN** tiers are derived
- **THEN** no path escalates a learning to `validated`
### Requirement: IP-FR-003 Migration preserves stored tier until next index

The v1→v2 migration that previously reset compiled pages to `draft` (`store/migrate.go:112`, current `schemaVersion = 3`) MUST NOT modify `pages.tier` for any existing rows. The migration MUST keep the stored tier value unchanged; the effective tier is re-derived only on the next index per IP-FR-001 (GTS-FR-001 blob equality + GTS-FR-003 downgrade). A fresh database built from a vault whose compiled pages are `validated` on the default branch MUST ingest as `validated` via IP-FR-001. Learnings migration behavior is unchanged (learnings remain `draft` per IP-FR-002).

#### Scenario: Fresh ingest after migration respects git trust

- **GIVEN** a vault on `origin/main` with `compiled/auth.md` at `tier: validated` and matching blob
- **WHEN** a new `graph.db` is built after the migration change
- **THEN** `pages.tier` for that page is `validated`

#### Scenario: Upgrade preserves tier until next index

- **GIVEN** an existing DB at schema v2 with a compiled page row at `tier: validated`
- **WHEN** the process migrates to v3
- **THEN** the migration itself leaves `pages.tier` unchanged; the row is not reset to `draft` — its effective tier after the next index reflects IP-FR-001 (remains `validated` if file frontmatter is canonical `validated` and blob matches default branch, otherwise `draft`)
### Requirement: IP-FR-004 Default-branch ref resolution is offline and deterministic

The pipeline MUST resolve the comparison ref from local git state only (offline): the fetched remote-tracking ref for the configured default branch (default `origin/main`), without network fetch. The effective `<remote>/<default-branch>` value MUST be resolved in exactly one precedence order, highest first: per-vault `.uf/dewey/config.yaml` → global `~/.config/dewey/config.yaml` → environment variable `DEWEY_DEFAULT_BRANCH_REF` → git config (`dewey.defaultBranchRef`) → default `origin/main`; the first source providing a value wins. The environment variable value grammar is `<remote>/<branch>` (for example `origin/main`, `upstream/main`) and MUST be validated per `git check-ref-format` before use like any other leg. The precedence leg that supplied the effective ref (or that failure fell back to draft) MUST be recorded in diagnostics observable in logs and via `dewey doctor`. The configured remote and branch MUST be the same values used to determine the protected default branch, and resolution MUST be deterministic for a given local ref state. Failure to resolve MUST trigger the non-git fallback (GTS-FR-004).

The configured `<remote>/<default-branch>` value MUST be validated before any git invocation, per `git check-ref-format` rules: bounded length, no leading `-` (flag-like values such as `--upload-pack=evil` or `-h`), no whitespace or control characters, no `..`, and no other characters that could alter git argument parsing. All git plumbing (`ls-tree`, `cat-file`, `hash-object`) MUST be invoked via argv arrays with `--` argument separators — never through a shell string. A value failing validation MUST be treated as unresolvable: the affected source degrades to the draft fallback (GTS-FR-004) with a diagnostic naming the invalid configuration; the value MUST NOT reach git and MUST NOT fail ingestion.

#### Scenario: Offline ref resolution
- **GIVEN** a git vault with `origin/main` fetched but no network available
- **WHEN** ingestion needs the default-branch blob for a path
- **THEN** it resolves the blob from the local `origin/main` ref without fetching
#### Scenario: Unresolvable ref falls back to draft

- **GIVEN** `origin/main` does not exist locally and no default-branch ref is configured as resolvable
- **WHEN** ingestion derives tiers
- **THEN** all compiled/curated pages are treated as `draft` per GTS-FR-004 and a diagnostic notes the missing ref

#### Scenario: Malformed configured ref is rejected before git runs

- **GIVEN** a git vault whose configured default-branch ref value is malformed (e.g. `--upload-pack=evil`, `-h`, contains whitespace or control characters, or contains `..`)
- **WHEN** ingestion derives tiers
- **THEN** the value is rejected before any git invocation, the affected pages ingest as `draft` per GTS-FR-004, and a diagnostic names the invalid configured ref

<!-- scaffolded by uf vdev -->
