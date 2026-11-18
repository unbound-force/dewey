# Design: Record Which Dewey Built an Index, and Say When It Needs a Reindex

## Context

The persistent store's `metadata` table (`store/migrate.go`,
`createSchema`) holds a single key, `schema_version`. Nothing records
which binary built the index, so after an upgrade that changes what
indexing stores (#220 in 3.8.0), operators and gh-hud cannot tell which
vaults still need `dewey reindex`.

The version plumbing that exists today cannot answer it either:

- `main.go` declares `var version = "dev"`, set by release ldflags
  (GoReleaser via the org-infra reusable workflows). `dewey version`
  (`newVersionCmd`) and the serve startup log print this var verbatim.
- mise `go:` installs are built with `go install <module>@<tag>`, which
  stamps the tag into `debug.ReadBuildInfo().Main.Version` but not into
  ldflags — so they report `dev`.
- `dewey status` (`cli.go`: `statusData`, `queryStoreStatus`,
  `formatStatusJSON`) opens the store and reports counts, sources, and
  trust state; it has no index-build fields.
- Index-building passes: `newIndexCmd` and `newReindexCmd` in `cli.go`
  (both end with `vault.IndexDocuments`), the serve startup
  `deferredIndex` closure in `main.go` (learning re-ingestion,
  knowledge-store re-index, `indexVault`, external-page load), and the
  `index`/`reindex` MCP tools (`tools/indexing.go`, constructed in
  `server.go`).
- `store.Store` already exposes generic `GetMeta`/`SetMeta` over the
  `metadata` table, both tested.

The design gate from epic unbound-force/dewey#226 was settled in #224's
comments: **an index format number** — a constant in the code,
`IndexFormat`, starting at 1, bumped by any PR that changes what
indexing stores; `index_format` and `dewey_version` written to
`metadata`; `dewey status --json` reporting both plus
`reindexRecommended` and `reindexReasons`; a missing `index_format`
counts as 0; recommend when stored is lower than the binary's. A list
of release versions was rejected because mise `go:` builds report
`dev`. This design implements that decision as written.

Constitution alignment is assessed in the proposal; the decisions below
flag where each principle shaped the design.

## Goals / Non-Goals

### Goals

- Record, in the index itself, which Dewey version and index format
  last completed an indexing pass over it (IBP-FR-001).
- Resolve the binary's version from ldflags or build info so mise `go:`
  installs report their real tag (IBP-FR-002).
- Report the stamp plus a one-field reindex recommendation in
  `dewey status --json` for gh-hud's teardown check (IBP-FR-003).
- Establish the `IndexFormat` bump discipline, documented in AGENTS.md
  (IBP-FR-004).
- Zero schema change, zero re-embedding, zero startup-behavior change
  for unaffected vaults (epic #226 rules).

### Non-Goals

- No automatic reindex — the output is a recommendation; the operator
  (or gh-hud) decides.
- No release-version comparison anywhere — the format number is the
  only comparison signal (settled decision).
- No `schema_version` bump or migration — new `metadata` keys only,
  which the epic explicitly allows.
- No change to the learning file format, frontmatter, or re-ingestion
  behavior.
- No retroactive format assignment for past releases — `IndexFormat`
  starts at 1; existing indexes count as 0 and are flagged once.
- No changes to gh-hud — it reads `reindexRecommended` when it adopts
  the field.
- Not making `dewey status` heal anything — status stays read-only.

## Decisions

### D1: `IndexFormat` constant in package main

`main.go` gains `IndexFormat = 1` with a doc comment stating the bump
rule: any PR that changes what indexing stores MUST bump it, and is
expected to make the incremental passes converge existing stores when
feasible (the #220 pattern: backfill on the unchanged-content path), or
rely on the reindex recommendation as the remediation.

Rationale: the settled comment names the constant and its start value.
Package main owns it because every stamp site (CLI commands, startup
closure) and the status comparison live in package main (`main.go` /
`cli.go`), and no other package may import package main. Unexported
would suffice for tests in package main; the exported name matches the
settled decision text and costs nothing (package main has no importers).

### D2: Version resolution as a pure seam

```go
// resolveVersionFrom returns the version identifying the binary:
// the ldflags version when set, else the build-info module version,
// else "dev".
func resolveVersionFrom(ldflagsVersion string, bi *debug.BuildInfo) string
func resolveVersion() string // wraps the real inputs
```

Order: ldflags value when neither empty nor `dev`; else
`bi.Main.Version` when neither empty nor `(devel)`; else `dev`.
`dewey version`, the serve startup log, and the stamp all use
`resolveVersion()`.

Rationale (Testability, constitution IV): the real inputs are baked at
link time and cannot vary in a unit test, so the decision is a pure
function over injected inputs with table-driven tests; the wrapper is a
one-liner. The resolved value is recorded and displayed **verbatim** —
ldflags builds record `3.9.0`, build-info builds record `v3.8.1` — and
is never parsed, so the prefix inconsistency is cosmetic (settled
design compares format numbers, not versions). Only stdlib
`runtime/debug` is used (Composability: no new dependency, no CGO).

### D3: Stamping via a package-main helper over `SetMeta`

```go
// recordIndexBuild writes dewey_version and index_format to the
// store's metadata table. Nil store is a no-op (in-memory mode).
func recordIndexBuild(s *store.Store, deweyVersion string, indexFormat int) error
```

Two `SetMeta` upserts on the already-open store, plus
`strconv.Itoa`. The store package stays generic (a metadata table of
key-value pairs); the domain meaning of these two keys lives in package
main next to the constant that defines one of them (Single
Responsibility). No transaction: the worst interleaving leaves
`index_format` missing, which counts as 0 and over-recommends — the
fail-safe direction (see Risks).

### D4: Stamp sites and semantics

`index_format` means **the `IndexFormat` of the code that last
completed an indexing pass over this store**. The helper is called at
five sites, each after that site's index work succeeds:

| Site | File | On stamp failure |
|---|---|---|
| `dewey index` | `cli.go` `newIndexCmd` RunE, after the `indexErr` check | fail the command (wrapped error) |
| `dewey reindex` | `cli.go` `newReindexCmd` RunE, after the `indexErr` check | fail the command |
| serve startup passes | `main.go` `deferredIndex`, after all passes, before `return nil` | warn and keep serving |
| MCP `index` tool | `tools/indexing.go` `Index`, after `IndexDocuments` | error result; index work stands |
| MCP `reindex` tool | `tools/indexing.go` `Reindex`, after its rebuild | error result; index work stands |

The MCP tools are included beyond the settled comment's three sites
because they are index-building paths inside a running server; leaving
them unstamped means an MCP-triggered reindex leaves a stale
recommendation forever. They receive the stamp values by constructor
injection: `NewIndexing` gains a final parameter — a small exported
value struct (`DeweyVersion string`, `IndexFormat int`) — supplied by
`server.go` from `resolveVersion()` and `IndexFormat`. Data injection,
not a behavior seam, keeps handler logic unchanged and assertions
trivial (Testability). Construction sites are `server.go` and the
tools tests.

Stamping after an incremental pass that changed nothing is intentional
and truthful under the stated semantics: the pass ran with this
binary's code, and a bump PR's convergence duty (D1) is what makes
"the pass ran" mean "the store is current". `dewey reindex` remains
the guaranteed clean slate. Serve failure handling keeps epic rule
"dewey serve startup behavior ... stays the same": the stamp adds two
upserts after the passes and a warn log at worst; it MUST NOT
re-persist or re-embed anything (IBP-FR-001), so unchanged files stay
cheap.

`dewey status` never calls the helper — the stamp is write-only at
index-build time (Observable Quality: provenance records the builder,
not the observer).

### D5: Status reporting

`queryStoreStatus` (`cli.go`), which already opens the store, reads
`GetMeta("dewey_version")` and `GetMeta("index_format")` and populates
new `statusData` fields: `DeweyVersion string`, `IndexFormat int`,
`ReindexRecommended bool`, `ReindexReasons []string`.

Parsing and the decision are pure functions:

```go
// parseIndexFormat returns the stored index format; missing or
// non-decimal values count as 0.
func parseIndexFormat(s string) int
// reindexDecision returns whether a reindex is recommended and why.
// Recommended iff stored < current.
func reindexDecision(storedFormat, currentFormat int) (bool, []string)
```

The reason is exactly `index format <stored>, this Dewey writes
<current>` (settled wording). `formatStatusJSON` emits four new
top-level keys — `deweyVersion`, `indexFormat`, `reindexRecommended`,
`reindexReasons` — always present (empty array when not recommended)
so consumers parse a deterministic shape (Autonomous Collaboration:
gh-hud reads one field, `reindexRecommended`, with no inference).
camelCase matches the existing top-level status keys
(`embeddingModel`, `embeddingCoverage`). The text output gains one
`Indexed by:` line and, when recommended, one `Reindex:` line carrying
the reason (IBP-FR-005).

Edge rules (all settled or proposed-default, encoded in
`reindexDecision`/`parseIndexFormat`): missing or unparseable stored
format counts as 0; stored strictly lower than the binary's format is
recommended; equal or higher is not (the hard compatibility gate for
unreadable stores remains `schema_version`); no graph.db yields empty
version, format 0, and **no** recommendation — there is nothing to
rebuild, and `dewey status` already reports zero counts for that case.

## Risks / Trade-offs

- **Every existing index is flagged once.** Indexes with no
  `index_format` count as 0, including ones already rebuilt by 3.8.0.
  Settled in #224 comments: one extra reindex beats a special case.
- **An incremental pass can clear the recommendation without a full
  rebuild.** Under D4's semantics this is correct only when bump PRs
  converge the store in the incremental paths; the AGENTS.md rule
  (IBP-FR-004) is the enforcement point, and #220's properties
  backfill is the template. If a bump PR skips convergence, affected
  vaults read "current" while carrying older per-page representations
  — the residual risk of the decided no-special-case design.
- **Two-key write is not atomic.** A crash between the upserts leaves
  `dewey_version` set and `index_format` missing → counts as 0 →
  over-recommends. Fail-safe direction; accepted.
- **Stored format higher than the binary's is not flagged.** Strictly
  per the settled rule; a downgrade scenario also re-stamps the older
  format on its next pass. `schema_version` still hard-fails genuinely
  unreadable stores.
- **Version strings are not normalized.** `3.9.0` (ldflags) vs
  `v3.8.1` (build info) vs `dev` (local builds) all get recorded and
  displayed as-is. Acceptable because nothing parses them; a `dev`
  stamp is honest provenance for a local build.
- **`NewIndexing` signature change** touches `server.go` and the tools
  tests. Contained to package-internal construction sites; no external
  callers exist outside this repo.
- **Status text output changes** by one or two lines. Existing status
  tests assert on that output and are updated in the same change; JSON
  consumers see only additive keys.

Constitution reference (from the proposal): Autonomous Collaboration —
gh-hud consumes a self-describing JSON field instead of out-of-band
binary-path knowledge; Composability First — stdlib-only, standalone,
in-memory mode unaffected; Observable Quality — the index carries its
own build provenance with deterministic absent-value rules;
Testability — every decision is a pure function over injected inputs.
