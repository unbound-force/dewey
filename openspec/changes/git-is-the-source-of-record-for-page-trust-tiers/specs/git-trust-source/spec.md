# Capability: git-trust-source

## ADDED Requirements

### Requirement: GTS-FR-001 Git blob equality determines validated tier for compiled/curated pages

The system MUST derive `tier: validated` for a compiled or curated page if and only if both conditions hold: (a) the file's frontmatter declares `tier: validated` as a canonical match — the `tier` value trimmed of leading/trailing whitespace and compared case-sensitively to the exact lowercase string `validated` (so `tier: Validated`, `tier: VALIDATED`, or `tier: validated ` with trailing whitespace variants are not matches and MUST be treated as unknown), and (b) the file's blob object ID equals the blob at `<remote>/<default-branch>` for the same path as resolved from the locally fetched ref. If either condition fails the effective tier MUST NOT be `validated`. The canonical tier vocabulary is `authored`, `curated`, `validated`, `draft`, and `untrusted` (each compared as the `tier` value trimmed of leading/trailing whitespace and case-sensitively equal to the exact lowercase string); these values MUST be preserved as declared and MUST NOT be degraded to `draft` by the gate. Any `tier` value outside this canonical set is unknown — it MUST degrade to `draft` per GTS-FR-003 and MUST NOT raise trust — so raising trust remains exclusively GTS-FR-001's blob-equality grant.

#### Scenario: Identical to default branch is validated
- **GIVEN** a compiled page `compiled/auth.md` whose file contains `tier: validated` and whose blob SHA equals the blob at `origin/main` for that path
- **WHEN** ingestion derives the effective tier
- **THEN** the ingested page tier is `validated`

#### Scenario: Frontmatter alone does not validate without blob match
- **GIVEN** a compiled page whose file contains `tier: validated` but whose blob differs from `origin/main` (regenerated content not yet merged)
- **WHEN** ingestion derives the effective tier
- **THEN** the ingested page tier is `draft` (GTS-FR-002 applies) and the file is not treated as vouched

### Requirement: GTS-FR-002 Raising trust comes only from the default branch

The system MUST treat any attempt to raise trust outside the default branch as `draft`. Specifically, a working-tree edit, an unmerged branch commit, or a stale fetched ref whose blob differs from the current default-branch blob for the same path MUST NOT yield `validated`, even when frontmatter says `tier: validated`.

#### Scenario: Working-tree edit cannot self-promote
- **GIVEN** a compiled page with `tier: validated` in the working tree that differs from `origin/main`
- **WHEN** the local server indexes the vault
- **THEN** the effective tier is `draft`

#### Scenario: Branch-only promotion stays draft until merged
- **GIVEN** `dewey promote` has set `tier: validated` in a file on a feature branch and the change is not merged to the default branch
- **WHEN** any server indexes that branch's tip
- **THEN** the effective tier is `draft` until the PR merges and the fetched default-branch ref contains the new blob

#### Scenario: Regenerated article without re-merge is draft (fixes #204)
- **GIVEN** a compiled article previously merged as `validated` at blob A and regenerated locally to blob B without a new merge
- **WHEN** ingestion compares blob B to the default-branch blob A
- **THEN** the effective tier is `draft`

### Requirement: GTS-FR-003 Lowering trust is honored everywhere

The system MUST honor lowering of trust regardless of git location or branch. A page MUST be ingested at the lowered tier/validity even when the default-branch blob matches when either authoritative downgrade signal holds: (a) the page's `validity` (frontmatter or derived, e.g. by synthesis) is one of `stale-unverified`, `superseded`, or `historical` under canonical matching — the value trimmed of leading/trailing whitespace and compared case-sensitively to the exact lowercase vocabulary (so `Stale-Unverified`, `SUPERSEDED` or padded variants are unknown), or (b) the page's frontmatter declares `tier: draft` under the same canonical matching (`tier` trimmed and case-sensitive equal to `draft`). These two signals are the exhaustive downgrade predicate — no other field or value lowers trust. This override is fail-safe and applies to vault files, branch files, and local edits.

Unknown or unrecognized values MUST NOT raise trust: a `validity` value outside the canonical enumerated set (including the empty or absent case), a `tier` value outside the canonical vocabulary `authored`/`curated`/`validated`/`draft`/`untrusted` (including non-canonical case variants like `Validated` or `Curated`), or any other frontmatter field MUST never upgrade a page to `validated` — raising trust is exclusively GTS-FR-001's blob-equality grant. Canonical matching is the sole vocabulary gate: values are trimmed and compared case-sensitively to the exact lowercase set; canonical non-validated tiers (`authored`, `curated`, `untrusted`) are preserved as declared, while variants outside the canonical set are unknown and MUST degrade to `draft`.

#### Scenario: Stale validated page is not validated
- **GIVEN** a compiled page whose file contains `tier: validated` and `validity: stale-unverified` (or is marked stale-unverified by synthesis)
- **WHEN** ingestion derives the effective tier/validity
- **THEN** the effective tier is not `validated` (downgrade honored) even if blob equals default branch, and the `stale-unverified` validity is preserved
#### Scenario: Frontmatter downgrade respected on any branch

- **GIVEN** a compiled page whose file contains `tier: draft` (downgrade from prior `validated`)
- **WHEN** ingestion runs on any checkout or branch
- **THEN** the effective tier is `draft` regardless of default-branch blob

#### Scenario: Unknown validity value cannot raise trust past the git rule

- **GIVEN** a compiled page whose blob differs from the default branch (git rule yields `draft`) whose file carries an unrecognized `validity: super-validated`
- **WHEN** ingestion derives the effective tier
- **THEN** the effective tier remains `draft` — unknown values are not upgrade signals; only GTS-FR-001's blob-equality grant raises trust

#### Scenario: Non-canonical tier case variant does not validate

- **GIVEN** a compiled page `compiled/auth.md` whose file contains `tier: Validated` (capital V) or `tier: VALIDATED` and whose blob equals `origin/main` for that path
- **WHEN** ingestion derives the effective tier
- **THEN** the effective tier is `draft` — canonical matching requires exact lowercase `validated`; the variant is unknown and MUST NOT raise trust even with blob equality

#### Scenario: Curated tier preserved
- **GIVEN** a curated page `knowledge/team-notes.md` whose file contains `tier: curated` (or `authored`/`untrusted`) and whose blob equals `origin/main` for that path
- **WHEN** ingestion derives the effective tier
- **THEN** the effective tier is `curated` (respectively `authored`/`untrusted`) — canonical non-validated tiers are preserved as declared and MUST NOT be degraded to `draft` by the gate; only `validated` requires blob equality per GTS-FR-001

### Requirement: GTS-FR-004 Non-git fallback is draft

When the vault is not a git repository, no default-branch ref (`<remote>/<default-branch>`) is resolvable from local git state, OR any git derivation step fails — missing git binary, permission denied, corrupt object database, malformed or truncated `ls-tree`/`cat-file` output, a non-zero exit that does not mean "ref not found", or a hung process — the system MUST treat the affected page or source as `draft` for trust purposes. A git derivation failure MUST NOT fail ingestion and MUST NOT yield `validated`. Every git invocation MUST run under a bounded context timeout — 5 seconds per invocation and 60 seconds cumulative per source, following the precedent in `tools/lint_anchors.go` — so a hung git process cannot stall indexing; timeout expiry MUST degrade the affected pages to `draft` with a diagnostic. The bounded context's cancellation MUST be propagated to the git child process so it does not outlive the timeout. Each fallback MUST emit a diagnostic recording the cause, observable in logs and via `dewey doctor`. The system MUST NOT require network access; this is the fail-safe extension of the current observable fallback behavior.

#### Scenario: Non-git vault fallback
- **GIVEN** a vault at a plain directory with no `.git`
- **WHEN** ingestion derives tiers
- **THEN** every compiled/curated page effective tier is `draft`
#### Scenario: No default-branch ref available

- **GIVEN** a git vault where `origin/main` (or configured `<remote>/<default-branch>`) cannot be resolved (no fetch, detached, shallow without ref)
- **WHEN** ingestion derives tiers
- **THEN** every compiled/curated page effective tier is `draft` until a fetch provides the ref

#### Scenario: Git derivation failure degrades to draft with diagnostic

- **GIVEN** a git vault where the git binary is missing from `PATH`, `.git/objects` is unreadable, or `git ls-tree` exits non-zero, emits malformed output, or hangs past the bounded timeout
- **WHEN** ingestion derives tiers
- **THEN** the affected source's compiled/curated pages ingest as `draft`, ingestion completes without failing, and a diagnostic records the git failure cause

### Requirement: GTS-FR-005 Database tier is a cache of the git rule

The `pages.tier` column (and `blocks` trust metadata derived from it) MUST be a cached projection of the git rule in GTS-FR-001 through GTS-FR-004, not the source of record. The git blob equality check MUST be the authority; the database MUST be recomputable from file state plus fetched refs. Two servers with different SQLite files but the same fetched default-branch refs MUST derive the same effective tiers for the same file set.

#### Scenario: Two servers with different DBs agree on tier
- **GIVEN** two Dewey servers A and B with independently built `graph.db` files but the same fetched `origin/main` refs and the same compiled article files
- **WHEN** each server indexes
- **THEN** the effective tier for each compiled/curated page is identical on A and B (determined by blob equality, not prior DB content)

#### Scenario: Learning tier never escalates even on default branch
- **GIVEN** a learning file `learning/foo-20260502T143022-alice.md` claiming `tier: validated` and whose blob equals `origin/main` for that path
- **WHEN** ingestion derives the tier
- **THEN** the effective tier remains `draft` per IP-FR-002 (learnings stay `draft` forever; GTS-FR-005 does not promote them)

<!-- scaffolded by uf vdev -->
