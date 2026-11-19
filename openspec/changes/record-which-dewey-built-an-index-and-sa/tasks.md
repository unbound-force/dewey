<!--
  [P] marks tasks eligible for parallel execution.
  Tasks without [P] run sequentially first, then [P] tasks
  run in parallel. [P] tasks touch different files and have
  no ordering constraints between them.
-->

## 1. Version resolution and IndexFormat constant

- [x] 1.1 `main.go`: add the `IndexFormat` constant with value `1`
  and a doc comment stating the bump rule — any PR that changes what
  indexing stores MUST bump it and is expected to make the incremental
  passes converge existing stores when feasible (IBP-FR-004, design
  D1).
- [x] 1.2 `main.go`: add `resolveVersionFrom(ldflagsVersion string, bi
  *debug.BuildInfo) string` (ldflags non-`dev`/non-empty → build-info
  `Main.Version` non-`(devel)`/non-empty → `dev`) and the
  `resolveVersion()` wrapper; `dewey version` (`newVersionCmd`) and
  the serve startup log report the resolved version (IBP-FR-002,
  design D2).

## 2. Index-build stamping

- [x] 2.1 `main.go`: add `recordIndexBuild(s *store.Store,
  deweyVersion string, indexFormat int) error` — two `SetMeta`
  upserts, nil store is a no-op, no page/block/embedding writes
  (IBP-FR-001, design D3).
- [x] 2.2 [P] `cli.go`: call `recordIndexBuild(s, resolveVersion(),
  IndexFormat)` in `newIndexCmd` and `newReindexCmd` after the
  `indexErr` check; a stamp failure fails the command with a wrapped
  error (IBP-FR-001, design D4).
- [x] 2.3 [P] `main.go`: call `recordIndexBuild` at the end of
  `deferredIndex` (guarded by `persistentStore != nil`), warn-only on
  failure so serve startup behavior is otherwise unchanged
  (IBP-FR-001, design D4).
- [x] 2.4 [P] `tools/indexing.go` + `server.go`: add the
  `IndexBuildInfo` value struct (`DeweyVersion`, `IndexFormat`), pass
  it to `NewIndexing` as a final parameter from `server.go`
  (`resolveVersion()`, `IndexFormat`), and stamp in the `Index` and
  `Reindex` handlers after their index work succeeds, reporting a
  stamp failure in the error result (IBP-FR-001, design D4).

## 3. Status reporting

- [x] 3.1 `cli.go`: add pure helpers `parseIndexFormat(s string) int`
  (missing or non-decimal counts as 0) and
  `reindexDecision(storedFormat, currentFormat int) (bool, []string)`
  (recommended iff stored < current; reason `index format <stored>,
  this Dewey writes <current>`) (IBP-FR-003, design D5).
- [x] 3.2 `cli.go`: `queryStoreStatus` reads `dewey_version` and
  `index_format` via `GetMeta` and populates new `statusData` fields
  (`DeweyVersion`, `IndexFormat`, `ReindexRecommended`,
  `ReindexReasons`); the no-graph.db path returns empty version,
  format 0, and no recommendation; status never writes either key
  (IBP-FR-003, design D5).
- [x] 3.3 `cli.go`: `formatStatusJSON` emits top-level `deweyVersion`,
  `indexFormat`, `reindexRecommended`, and `reindexReasons` (always
  present; empty array, not null, when not recommended) (IBP-FR-003,
  design D5); `formatStatusText` gains an `Indexed by:` line and, when
  recommended, a `Reindex:` line with the reason (IBP-FR-005, design
  D5).

## 4. Tests

- [x] 4.1 [P] `main_test.go`: `TestResolveVersionFrom` table tests
  (ldflags release value, build-info module version, `(devel)`,
  empty/nil inputs) and `TestRecordIndexBuild` (both keys written,
  nil store no-op, values verbatim).
- [x] 4.2 `cli_test.go`: `TestParseIndexFormat` and
  `TestReindexDecision` table tests (lower, missing → 0, equal,
  higher, unparseable); extend the status command tests to assert the
  four JSON fields across scenarios (older format with the exact
  reason string, missing key, current, newer, unparseable, no
  graph.db, status leaves metadata unwritten) and the new text-output
  lines; assert `dewey index` and `dewey reindex` stamp a temp-dir
  vault (disk source, `--no-embeddings`) (IBP-FR-001..005).
- [x] 4.3 [P] `tools/indexing_test.go`: assert the `Index` and
  `Reindex` MCP handlers stamp an injected `IndexBuildInfo` on
  success (IBP-FR-001).

## 5. Documentation

- [x] 5.1 `AGENTS.md`: add the `IndexFormat` bump rule to the PR
  checklist (Technical Guardrails section) and update the `dewey
  status` row to mention the recorded version and reindex
  recommendation (IBP-FR-004, documentation validation gate).
- [x] 5.2 [P] File the website sync issue per the AGENTS.md
  documentation gate: `gh issue create --repo unbound-force/website
  --label docs` titled `docs: sync dewey index-version documentation`,
  covering the new `dewey status --json` fields, the reindex
  recommendation, and `dewey version` build-info resolution.

## 6. Verification

- [ ] 6.1 CI parity: derive the exact commands from
  `.github/workflows/ci.yml` and run them locally — build, vet,
  `go test -race -count=1 -coverprofile=coverage.out ./...`, and the
  Gaze threshold gate. All must pass before the PR.
- [ ] 6.2 Constitution alignment verification: confirm each principle
  — Autonomous Collaboration (gh-hud reads one JSON field, no runtime
  coupling), Composability First (stdlib-only, standalone, in-memory
  mode unaffected), Observable Quality (index carries its own build
  provenance with deterministic absent-value rules), Testability
  (pure seams over injected inputs; table-driven tests; no external
  services).
- [ ] 6.3 Review council (`/uf.review-council`) — resolve all
  REQUEST CHANGES findings until all reviewers return APPROVE.
