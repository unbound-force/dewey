# Proposal: Record Which Dewey Built an Index, and Say When It Needs a Reindex

## Why

The `metadata` table in `.uf/dewey/graph.db` holds only `schema_version`
(`store/migrate.go`, `createSchema`). Nothing records which Dewey binary
built an index, so after an upgrade that changes what indexing stores
(#220, shipped in 3.8.0, is one), there is no way to tell which vaults
still need `dewey reindex`.

`dewey version` cannot answer this either: it prints only the
ldflags-injected `version` var (`main.go`, `var version = "dev"`), which
is `dev` for every non-GoReleaser build. In particular, mise `go:`
installs — built with `go install <module>@<tag>`, which stamps the tag
into `debug.ReadBuildInfo().Main.Version` but not into ldflags — report
`dev`. gh-hud currently guesses by remembering the resolved binary path
per checkout, which only works from the first time it looks
(jflowers/gh-hud#138).

The design gate from epic unbound-force/dewey#226 ("how a release
declares that it changes indexing") is settled in #224's comments:
**an index format number**. A constant in the code, `IndexFormat`,
starting at 1, bumped by any PR that changes what indexing stores. A
list of release versions would not work — mise `go:` builds report
`dev`, so release-version comparison has nothing reliable to compare
against. An integer format number is written by the binary itself and
compares without parsing.

## What Changes

1. **`IndexFormat` constant (main.go, package main).** Starts at `1`.
   Any PR that changes what indexing stores MUST bump it. The rule is
   added to AGENTS.md (PR checklist) so review catches a missed bump.

2. **Version resolution (main.go).** A `resolveVersion()` helper:
   the ldflags version when it is set to anything other than `dev` or
   empty; otherwise the module version from
   `debug.ReadBuildInfo().Main.Version` when it is set to anything
   other than `(devel)` or empty; otherwise `dev`. `dewey version`
   and the serve startup log print the resolved version, so mise
   `go:` installs report their real tag.

3. **Index-build stamping.** `dewey index`, `dewey reindex`, the
   serve startup passes, and the `index`/`reindex` MCP tools write two
   keys to the existing `metadata` table on pass completion:
   - `dewey_version` — the resolved version of the binary that ran
     the pass (informational; never parsed for comparison).
   - `index_format` — the value of `IndexFormat` in that binary
     (the comparison signal).

   Stamping is two `SetMeta` upserts on an already-open store. It MUST
   NOT re-embed or re-persist any page: unchanged files stay cheap at
   startup (epic #226 rule). No schema change and no `schema_version`
   bump — new keys in the `metadata` table are explicitly allowed by
   the epic. The learning file format and frontmatter are untouched.

4. **`dewey status` reports the stamp and a reindex recommendation.**
   `dewey status --json` gains four top-level fields:
   - `deweyVersion` — the stored `dewey_version` (empty when absent).
   - `indexFormat` — the stored `index_format` (see counting rule
     below).
   - `reindexRecommended` — true when the stored `index_format` is
     lower than this binary's `IndexFormat`.
   - `reindexReasons` — e.g. `["index format 1, this Dewey writes 2"]`,
     empty when no reindex is recommended.

   A stored `index_format` that is missing or unparseable counts as
   `0`, so every existing index is flagged exactly once on first
   contact with a format-aware binary — including indexes already
   rebuilt by 3.8.0. That one extra reindex is accepted over a special
   case (settled in #224 comments). The human-readable `dewey status`
   output gains one line showing the recorded version and, when
   recommended, the reason.

5. **Documentation.** AGENTS.md gains the `IndexFormat` bump rule in
   the PR checklist and the `dewey status` row notes the new fields.
   A website sync issue is filed per the AGENTS.md documentation gate.

### Scope decisions (reasonable defaults, recorded per epic instructions)

- **MCP `index`/`reindex` tools stamp too.** The settled comment names
  `dewey index`, `dewey reindex`, and the startup passes. The two MCP
  tools (tools/indexing.go) are also index-building paths inside a
  running server; leaving them unstamped means an MCP-triggered
  reindex leaves a stale `reindexRecommended` forever. All stamp sites
  live in package main or receive the stamp values from it, so the
  plumbing is two constructor fields.
- **Stamps record the pass, not a full rebuild.** `index_format`
  means "the `IndexFormat` of the code that last completed an
  indexing pass over this store". A PR that bumps `IndexFormat` is
  expected to make the incremental passes converge existing stores
  when feasible — the #220 pattern (backfill on the unchanged-content
  path) is the template — with `dewey reindex` remaining the
  guaranteed clean slate. This is the semantics that makes stamping
  after an incremental pass truthful.
- **`dewey status` never stamps.** Status is read-only; only
  index-building paths write the keys.
- **No graph.db, no recommendation.** When `.uf/dewey/` exists but
  `graph.db` does not, `reindexRecommended` is `false` (nothing to
  rebuild; any index command builds it).
- **Stored format higher than the binary's is not flagged.** The
  settled rule is strictly-lower. `schema_version` remains the hard
  compatibility gate for genuinely unreadable stores.
- **Stamp failure handling.** `dewey index`/`dewey reindex` fail the
  command when the stamp cannot be written (the command's job
  includes recording it). The serve startup pass logs a warning and
  keeps serving; the MCP tools report the stamp failure in their
  error result without discarding the index work.
- **`dewey version` prints the resolved version.** Strictly wider
  than the issue's ask, but the issue names `dewey version` reporting
  `dev` as part of the problem, and the resolution helper exists
  anyway.

## Capabilities

### New Capabilities
- `index-provenance`: The index records which Dewey version and index
  format built it, and `dewey status` reports that provenance plus a
  machine-readable reindex recommendation.

### Modified Capabilities
- None. (No existing spec under `openspec/specs/` covers the metadata
  table or status output.)

### Removed Capabilities
- None.

## Impact

- **Files**: `main.go` (constant, version resolution, startup stamp),
  `cli.go` (status fields, CLI index/reindex stamps, `dewey version`),
  `tools/indexing.go` + `server.go` (MCP tool stamps), `AGENTS.md`
  (bump rule, status row), tests alongside each.
- **Behavior**: Two metadata upserts after every completed indexing
  pass; four new JSON fields in `dewey status --json`; one new line in
  status text output; `dewey version` output changes for non-ldflags
  builds (`dev` → the build-info module version). Serve startup
  behavior for vaults that don't hit these cases is otherwise
  unchanged, and existing tests keep passing (epic #226 rules).
- **No schema change**: new `metadata` keys only; `schema_version`
  stays 3. No migration.
- **Consumers**: gh-hud's teardown check reads one field
  (`reindexRecommended`) instead of inferring from binary paths.

## Constitution Alignment

Assessed against the Unbound Force org constitution.

### I. Autonomous Collaboration

**Assessment**: PASS

The change improves artifact-based collaboration: gh-hud (and any
operator or agent) learns index freshness from `dewey status --json`
— a self-describing, machine-parseable output — instead of
out-of-band knowledge about binary paths. No runtime coupling is
introduced; the stamp is data in the index, not a protocol between
tools.

### II. Composability First

**Assessment**: PASS

Dewey remains fully standalone. The feature uses only stdlib
(`runtime/debug`) and the existing `metadata` table and `SetMeta`
path. No new dependency, no new service requirement, and existing
configs keep working unchanged. Vaults without a store, and in-memory
mode, are unaffected.

### III. Observable Quality

**Assessment**: PASS

The index gains provenance metadata about its own builder
(`dewey_version`, `index_format`), and `dewey status --json` exposes
it with a derived recommendation (`reindexRecommended` +
`reindexReasons`). Every field has a defined absent-value rule, so the
output is deterministic and auditable.

### IV. Testability

**Assessment**: PASS

Version resolution, the counts-as-zero parsing rule, and the
recommendation decision are pure functions testable with table-driven
tests and no external services. Stamp sites are exercised against a
temporary-dir store; status fields are asserted end-to-end through
the existing status command test patterns (`cli_test.go`).
