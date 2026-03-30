# Tasks: Unified Ignore Support

**Input**: Design documents from `/specs/006-unified-ignore/`
**Prerequisites**: plan.md ✓, spec.md ✓, research.md ✓, quickstart.md ✓, contracts/matcher.go ✓

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (e.g., US1, US2, US3, US4)
- Include exact file paths in descriptions

---

## Phase 1: Setup (Create Ignore Package)

**Purpose**: New `ignore/` package with Matcher type — zero dependencies on existing code

- [x] T001 Create `ignore/ignore.go` with `Matcher` struct, `pattern` type (value, negated, dirOnly, glob fields), and `NewMatcher(gitignorePath string, extraPatterns []string) (*Matcher, error)` constructor that reads `.gitignore` and parses patterns per contracts/matcher.go
- [x] T002 Implement `parsePatterns()` in `ignore/ignore.go` — parse `.gitignore` lines handling blank lines, comments (`#`), negation (`!`), directory patterns (trailing `/`), and file globs; use `filepath.Match()` for glob evaluation
- [x] T003 Implement `ShouldSkip(name string, isDir bool) bool` in `ignore/ignore.go` — evaluate hidden-directory baseline, negation patterns, gitignore patterns, and extra patterns in the order defined by contracts/matcher.go
- [x] T004 Implement `ShouldSkipPath(relPath string) bool` in `ignore/ignore.go` — split path into components, check each directory component and final filename against `ShouldSkip()`; used by file watcher event handler (R2)

**Checkpoint**: `ignore` package compiles and exports its full API surface. No integration yet.

---

## Phase 2: Foundational (Ignore Package Tests)

**Purpose**: Contract tests proving the Matcher works correctly before any integration

**⚠️ CRITICAL**: No integration work (Phases 3–6) can begin until these tests pass

- [x] T005 [P] Create `ignore/ignore_test.go` with `TestNewMatcher_*` tests: missing `.gitignore` returns valid matcher, empty `.gitignore`, malformed lines logged as warnings but parsing continues, file read errors
- [x] T006 [P] Add `TestShouldSkip_*` tests in `ignore/ignore_test.go`: directory patterns (`node_modules/`), file globs (`*.log`), negation (`!important.md`), hidden directories (`.git`), plain names, no-match returns false
- [x] T007 [P] Add `TestShouldSkipPath_*` tests in `ignore/ignore_test.go`: nested paths (`node_modules/pkg/README.md`), hidden dir in path (`.git/config`), clean paths (`docs/guide.md`), root-level files
- [x] T008 Add `TestShouldSkip_UnionMerge` test in `ignore/ignore_test.go`: verify gitignore patterns AND extra patterns both apply (union semantics per D4); negation in gitignore does not un-ignore extra patterns

**Checkpoint**: `go test -race -count=1 ./ignore/...` passes. Matcher contract is proven.

---

## Phase 3: US1 — .gitignore Respect in All Walkers (Priority: P1) 🎯 MVP

**Goal**: All four filesystem walkers respect `.gitignore` at the walk root — zero-config fix for the `node_modules/` timeout bug

**Independent Test**: Create vault with `.gitignore` containing `node_modules/`, place `.md` files inside, run `dewey serve`, verify excluded

### Implementation

- [x] T009 Add `ignorePatterns []string` field and `WithIgnorePatterns(patterns []string) Option` to vault `Client` struct in `vault/vault.go`; build `ignore.Matcher` in `Load()` from vault root `.gitignore` + `ignorePatterns`
- [x] T010 Update `Load()` walk callback in `vault/vault.go` to use `Matcher.ShouldSkip()` instead of inline `strings.HasPrefix(info.Name(), ".")` check
- [x] T011 Update `addWatcherDirs()` in `vault/vault.go` to use `Matcher.ShouldSkip()` instead of inline hidden-dir check; preserve the `path != root` guard (quickstart.md gotcha #3)
- [x] T012 Update `handleEvent()` in `vault/vault.go` to use `Matcher.ShouldSkipPath()` instead of `strings.Contains(event.Name, "/.")` check
- [x] T013 Update `walkVault()` signature in `vault/vault_store.go` to accept `*ignore.Matcher` parameter; update walk callback to use `Matcher.ShouldSkip()`; update caller `IncrementalIndex()` to pass the matcher (quickstart.md gotcha #1)

### Tests

- [x] T014 [P] Add `TestLoad_GitignoreRespected` in `vault/vault_test.go` — `t.TempDir()` with `.gitignore` containing `node_modules/`, verify files inside are not indexed
- [x] T015 [P] Add `TestLoad_NoGitignore` in `vault/vault_test.go` — `t.TempDir()` without `.gitignore`, verify all non-hidden dirs indexed (backward compatibility per acceptance scenario 2)

**Checkpoint**: `go test -race -count=1 ./vault/...` passes. US1 acceptance scenarios 1–2 verified.

---

## Phase 4: US2 — Sources.yaml Ignore Configuration (Priority: P2)

**Goal**: Disk sources support `ignore` field in `sources.yaml` — explicit control beyond `.gitignore`

**Independent Test**: Add `ignore: [drafts]` to disk source config, verify files in `drafts/` excluded

### Implementation

- [x] T016 Add `DiskSourceOption` type, `WithIgnorePatterns()` and `WithRecursive()` options, and `ignorePatterns`/`recursive` fields to `DiskSource` struct in `source/disk.go`; update `NewDiskSource()` to accept `opts ...DiskSourceOption` (R6)
- [x] T017 Update `DiskSource.List()` and `walkDiskFiles()` in `source/disk.go` to build `ignore.Matcher` from source root `.gitignore` + `ignorePatterns` and use it instead of inline hidden-dir check; also respect `recursive` flag (quickstart.md gotcha #4)
- [x] T018 Update `createDiskSource()` in `source/manager.go` to read `ignore` (via `extractStringList()` — gotcha #5) and `recursive` (bool) from `Config map[string]any` and pass as options to `NewDiskSource()`
- [x] T019 Update `validateSourceConfig()` in `source/config.go` to accept `ignore` (list) and `recursive` (bool) fields for disk source type without error

### Tests

- [x] T020 [P] Add `TestDiskSource_IgnorePatterns` in `source/disk_test.go` — `t.TempDir()` with `ignore: [drafts]`, verify files in `drafts/` excluded from `List()` results
- [x] T021 [P] Add `TestDiskSource_UnionMerge` in `source/disk_test.go` — source with both `.gitignore` and `ignore` patterns, verify both apply (acceptance scenario 2)

**Checkpoint**: `go test -race -count=1 ./source/...` passes. US2 acceptance scenarios 1–3 verified.

---

## Phase 5: US3 — Non-Recursive Source Indexing (Priority: P3)

**Goal**: `recursive: false` on a disk source indexes only top-level `.md` files

**Independent Test**: Set `recursive: false`, verify only root-level files indexed

- [x] T022 Add `TestDiskSource_RecursiveFalse` in `source/disk_test.go` — `t.TempDir()` with subdirectories containing `.md` files, `recursive: false`, verify only root-level files in `List()` results
- [x] T023 Add `TestDiskSource_RecursiveDefault` in `source/disk_test.go` — no `recursive` option set, verify all subdirectories traversed (backward compatibility per acceptance scenario 2)

**Checkpoint**: `go test -race -count=1 ./source/...` passes. US3 acceptance scenarios 1–3 verified.

---

## Phase 6: US4 + CLI Integration (Priority: P4)

**Goal**: File watcher consistency, `dewey init` template update, `dewey doctor` verbose ignore reporting, and main.go wiring

### Implementation

- [x] T024 Update `dewey init` template in `cli.go` to include `recursive: false` for sources with parent-directory paths (`"../"`) and add comment documenting `ignore`/`recursive` fields (FR-007, R7)
- [x] T025 Update `dewey doctor` in `cli.go` to accept `--verbose` flag; when verbose, build `Matcher` for vault path and report ignored directory count in Workspace section (FR-013, R8)
- [x] T026 Update `initObsidianBackend()` in `cli.go` to read `disk-local` source config from `sources.yaml`, extract `ignore` patterns, and pass to vault via `WithIgnorePatterns()` (R4)

### Tests

- [x] T027 [P] Add test for `dewey init` recursive default in `cli_test.go` — verify generated `sources.yaml` contains `recursive: false` for parent-directory sources
- [x] T028 [P] Add test for `dewey doctor` verbose ignore reporting in `cli_test.go` — verify ignored directory count appears in output when `--verbose` is set

**Checkpoint**: `go test -race -count=1 ./...` passes. US4 acceptance scenarios verified. All CLI changes tested.

---

## Phase 7: Verification (CI Parity Gate)

**Purpose**: End-to-end validation and CI-equivalent checks

- [x] T029 Add end-to-end test in `integration_test.go` — full pipeline: create vault with `.gitignore`, serve, verify excluded files not in index, verify watcher ignores events from excluded dirs
- [x] T030 Run CI parity gate: `go build ./... && go vet ./... && go test -race -count=1 ./...` — all must pass; verify no regressions in existing tests

**Checkpoint**: All tests pass. Feature is ready for review council.

---

## Dependencies & Execution Order

### Phase Dependencies

- **Phase 1 (Setup)**: No dependencies — start immediately
- **Phase 2 (Tests)**: Depends on Phase 1 — BLOCKS all integration phases
- **Phase 3 (US1)**: Depends on Phase 2 — can start after ignore package is proven
- **Phase 4 (US2)**: Depends on Phase 2 — can run in parallel with Phase 3 (different packages)
- **Phase 5 (US3)**: Depends on Phase 4 (T016–T017 add recursive support) — sequential after US2
- **Phase 6 (US4+CLI)**: Depends on Phases 3 and 4 — needs both vault and source integration complete
- **Phase 7 (Verification)**: Depends on all previous phases

### Parallel Opportunities

- **Phase 2**: T005, T006, T007 can run in parallel (same file, different test functions)
- **Phase 3 + Phase 4**: Can run in parallel (vault/ vs source/ — different packages, no file conflicts)
- **Within Phase 3**: T014, T015 can run in parallel with each other
- **Within Phase 4**: T020, T021 can run in parallel with each other
- **Within Phase 6**: T027, T028 can run in parallel with each other

### File Ownership (No Conflicts)

| File | Phase | Tasks |
|------|-------|-------|
| `ignore/ignore.go` | 1 | T001–T004 |
| `ignore/ignore_test.go` | 2 | T005–T008 |
| `vault/vault.go` | 3 | T009–T012 |
| `vault/vault_store.go` | 3 | T013 |
| `vault/vault_test.go` | 3 | T014–T015 |
| `source/disk.go` | 4 | T016–T017 |
| `source/manager.go` | 4 | T018 |
| `source/config.go` | 4 | T019 |
| `source/disk_test.go` | 4–5 | T020–T023 |
| `cli.go` | 6 | T024–T026 |
| `cli_test.go` | 6 | T027–T028 |
| `integration_test.go` | 7 | T029 |

---

## Notes

- All tests use `t.TempDir()` fixtures — no external services required
- The `ignore` package has zero dependencies beyond Go stdlib (`os`, `path/filepath`, `strings`, `bufio`)
- `filepath.Match()` handles glob patterns (`*`, `?`, `[...]`) — no custom glob engine needed
- The `Config map[string]any` stores YAML-parsed values: `ignore` arrives as `[]any`, not `[]string` — use `extractStringList()` helper (quickstart.md gotcha #5)
- `walkVault()` is a package-level function, not a method — signature change propagates to `IncrementalIndex()` caller (quickstart.md gotcha #1)
- Hidden-directory skipping is hardcoded in `Matcher` as a baseline — it cannot be overridden by patterns
<!-- spec-review: passed -->
