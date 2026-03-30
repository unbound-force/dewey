# Implementation Plan: Unified Ignore Support

**Branch**: `006-unified-ignore` | **Date**: 2026-03-30 | **Spec**: [spec.md](spec.md)
**Input**: Feature specification from `/specs/006-unified-ignore/spec.md`

## Summary

Add unified `.gitignore` and `sources.yaml` ignore support to all four filesystem walkers and the file watcher event handler. A new `ignore` package provides a shared `Matcher` type that evaluates directory/file patterns from `.gitignore` (root-level only), `sources.yaml` per-source `ignore` lists, and the existing hidden-directory baseline. The `sources.yaml` schema is extended with `ignore` (pattern list) and `recursive` (boolean) fields for disk sources. This eliminates the `node_modules/` timeout bug (374 junk files indexed) and prevents content duplication from parent-directory sources.

## Technical Context

**Language/Version**: Go 1.25 (per `go.mod`)
**Primary Dependencies**: `github.com/fsnotify/fsnotify` (file watcher), `github.com/spf13/cobra` (CLI), `github.com/charmbracelet/log` (logging), `gopkg.in/yaml.v3` (config parsing)
**Storage**: N/A (no storage changes — this feature modifies filesystem walking, not the SQLite store)
**Testing**: Standard library `testing` package with `t.TempDir()` fixtures, `-race -count=1`
**Target Platform**: darwin/linux (amd64/arm64)
**Project Type**: CLI + MCP server
**Performance Goals**: `dewey serve` startup < 5 seconds on projects with `node_modules/` in `.gitignore` (down from 40+ seconds)
**Constraints**: No CGO, no third-party gitignore libraries, local-only processing
**Scale/Scope**: 4 walker call sites + 1 event handler + 1 new package (~300 LOC production, ~400 LOC tests)

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

### I. Composability First — PASS

The `ignore` package is a pure Go package with zero external dependencies. It does not require any other Unbound Force tool. When no `.gitignore` exists and no `sources.yaml` ignore patterns are configured, behavior is identical to today (hidden-directory-only skipping). Dewey remains independently installable and usable.

### II. Autonomous Collaboration — PASS

No changes to MCP tool contracts. The ignore logic is internal to the filesystem walking layer — it does not affect MCP tool inputs, outputs, or the tool registry. All 40 existing MCP tools continue to work identically. The `dewey doctor` verbose output is additive (new diagnostic info, not a new MCP tool).

### III. Observable Quality — PASS

The `dewey doctor` command gains verbose-mode reporting of ignored directory counts, making the ignore behavior auditable. When `.gitignore` contains malformed lines, a warning is logged with the line number and content — the system does not silently swallow errors. The `dewey status` output is unchanged (page counts will naturally reflect fewer indexed files).

### IV. Testability — PASS

The `ignore` package is fully testable in isolation with no external dependencies — it operates on string patterns and directory names. All walker modifications are testable via `t.TempDir()` fixtures with synthetic `.gitignore` files. No Ollama, no SQLite, no network access required. Coverage strategy: contract tests for `Matcher.Match()` with pattern variants (directory, glob, negation, comment, blank), integration tests for each walker with `.gitignore` present/absent.

**Gate Result: ALL PASS — proceed to Phase 0.**

## Project Structure

### Documentation (this feature)

```text
specs/006-unified-ignore/
├── plan.md              # This file
├── research.md          # Phase 0 output — technical findings
├── quickstart.md        # Phase 1 output — implementation guide
├── contracts/           # Phase 1 output — interface contracts
│   └── matcher.go       # Matcher interface contract
└── tasks.md             # Phase 2 output (/speckit.tasks)
```

### Source Code (repository root)

```text
ignore/                  # NEW — shared ignore pattern matching
├── ignore.go            # Matcher type, pattern parsing, matching logic
└── ignore_test.go       # Contract tests for pattern matching

vault/
├── vault.go             # MODIFIED — Load(), addWatcherDirs(), handleEvent() use Matcher
└── vault_store.go       # MODIFIED — walkVault() uses Matcher

source/
├── disk.go              # MODIFIED — List(), walkDiskFiles() use Matcher; recursive support
├── config.go            # MODIFIED — validation for ignore/recursive fields
└── manager.go           # MODIFIED — createDiskSource() reads ignore/recursive from config

cli.go                   # MODIFIED — dewey init generates recursive:false for parent dirs;
                         #            dewey doctor reports ignored dir counts in verbose mode
```

**Structure Decision**: Flat package layout consistent with existing architecture. The new `ignore/` package sits alongside existing top-level packages (`vault/`, `source/`, `store/`, `embed/`, etc.). This follows the project's established pattern of one-package-per-concern. The `ignore` package is imported by both `vault` and `source`, eliminating code duplication (FR-012).

## Design Decisions

### D1: New `ignore` package vs. inline logic

**Decision**: Create a new `ignore/` package at the repository root.

**Rationale**: FR-012 requires shared ignore logic between `vault` and `source` packages. Inlining the logic in either package would create an import cycle or force duplication. A shared package follows the Composability First principle — it's independently testable and reusable.

**Alternatives rejected**:
- Putting it in `vault/` and importing from `source/` — creates a dependency from `source` → `vault` that doesn't exist today and violates separation of concerns
- Putting it in a `util/` or `internal/` package — the project doesn't use these patterns; flat layout is the convention

### D2: Minimal .gitignore parser vs. third-party library

**Decision**: Write a minimal ~50-line parser covering directory patterns, file globs, negation, comments, and blank lines.

**Rationale**: The constitution's Dependencies standard says "New dependencies MUST be justified by a clear need that cannot be met by the Go standard library." The patterns found in real Unbound Force repos (directory names like `node_modules/`, file globs like `*.log`, negation like `!important.md`) are trivially parseable. Full git pathspec semantics (`**/` double-star, leading `/` for root-relative) can be added later if needed.

**Alternatives rejected**:
- `github.com/sabhiram/go-gitignore` — adds an external dependency for ~30 lines of logic we can write ourselves
- `github.com/go-git/go-git` — massive dependency for a tiny feature

### D3: Root-level .gitignore only

**Decision**: Parse only the `.gitignore` at the root of the directory being walked. Do not process nested `.gitignore` files in subdirectories.

**Rationale**: FR-002 explicitly states this. Nested `.gitignore` processing adds significant complexity (stack-based pattern scoping during walk) for minimal benefit — the primary use case is skipping top-level directories like `node_modules/`, `vendor/`, `dist/`.

### D4: Union merge semantics for ignore sources

**Decision**: A path is ignored if ANY source says to ignore it (`.gitignore` OR `sources.yaml` ignore patterns OR hidden-directory rule). There is no way to "un-ignore" via `sources.yaml` what `.gitignore` already excludes.

**Rationale**: This is the simplest mental model. The spec's edge cases section explicitly states this: "`.gitignore` is the baseline, `sources.yaml` only adds exclusions." Negation patterns within `.gitignore` itself are respected (per git semantics), but cross-source negation is not supported.

### D5: Matcher built once per walk, not per-file

**Decision**: The `Matcher` is constructed once at the start of each walk operation from the union of all pattern sources. It is not rebuilt per-file or per-directory.

**Rationale**: Performance — reading and parsing `.gitignore` on every `filepath.Walk` callback would be wasteful. Building once and passing the `Matcher` into the walk function is the natural pattern.

### D6: Vault reads ignore config from `disk-local` source entry

**Decision**: The vault's `Load()`, `walkVault()`, and `addWatcherDirs()` functions read their `ignore` patterns from the `disk-local` source entry in `sources.yaml`. If no `disk-local` entry exists, only `.gitignore` and hidden-directory skipping apply.

**Rationale**: The vault already uses `sourceID: "disk-local"` (hardcoded in `WithStore()`). Reading the ignore config from the same source entry maintains consistency. The vault doesn't need its own separate ignore configuration mechanism.

### D7: `DiskSource` gains ignore/recursive fields via constructor

**Decision**: Extend `NewDiskSource()` to accept ignore patterns and recursive flag. The `createDiskSource()` factory in `manager.go` reads these from the `Config map[string]any`.

**Rationale**: The existing pattern uses `Config map[string]any` for type-specific config (see `source/config.go:18`). Adding `ignore` and `recursive` follows the same pattern. No schema change to `SourceConfig` struct is needed — the new fields live in the untyped config map.

### D8: `recursive: false` default for parent-directory sources in `dewey init`

**Decision**: When `dewey init` generates `sources.yaml`, sources with `path: "../"` (or equivalent parent references) get `recursive: false` by default.

**Rationale**: FR-007 requires this. Parent-directory sources are typically org-level aggregators that should only index top-level files, not recursively walk into sibling repos (which have their own sources).

## Implementation Approach

### Phase 1: Core ignore package

1. Create `ignore/ignore.go` with `Matcher` type
2. Implement `.gitignore` parser (directory patterns, file globs, negation, comments, blank lines)
3. Implement `Match(name string, isDir bool) bool` method
4. Implement `NewMatcher(gitignorePath string, extraPatterns []string) (*Matcher, error)` constructor
5. Write comprehensive contract tests in `ignore/ignore_test.go`

### Phase 2: Source package integration

1. Extend `NewDiskSource()` to accept ignore patterns and recursive flag
2. Update `DiskSource.List()` to use `Matcher` for directory/file filtering
3. Update `walkDiskFiles()` to use `Matcher`
4. Update `createDiskSource()` in `manager.go` to read `ignore`/`recursive` from config
5. Update `validateSourceConfig()` to accept (but not require) `ignore`/`recursive` fields
6. Write tests for `DiskSource` with ignore patterns and recursive=false

### Phase 3: Vault package integration

1. Add `Matcher` field to vault `Client` struct (built during `Load()`)
2. Update `Load()` to build `Matcher` from vault root `.gitignore` + `disk-local` source config
3. Update `walkVault()` to accept and use `Matcher`
4. Update `addWatcherDirs()` to use `Matcher`
5. Update `handleEvent()` to filter events from ignored paths
6. Write tests for vault walking with `.gitignore` present/absent

### Phase 4: CLI integration

1. Update `dewey init` to generate `recursive: false` for parent-directory sources
2. Update `dewey doctor` to report ignored directory counts in verbose mode
3. Write tests for init and doctor changes

### Phase 5: Integration testing

1. End-to-end test: vault with `.gitignore` → serve → verify excluded files
2. End-to-end test: disk source with `ignore` + `recursive: false` → index → verify
3. Backward compatibility: vault with no `.gitignore` → identical behavior

## Coverage Strategy

| Package | Test Type | What's Tested | Target |
|---------|-----------|---------------|--------|
| `ignore` | Contract | Pattern parsing, matching (directory, glob, negation, comment, blank, malformed) | 90%+ |
| `source` | Contract | `DiskSource.List()` with ignore patterns, recursive=false, config parsing | 80%+ |
| `vault` | Contract | `Load()` with `.gitignore`, `walkVault()` with matcher, `handleEvent()` filtering | 70%+ |
| `cli` | Contract | `dewey init` recursive default, `dewey doctor` verbose output | 70%+ |
| root | Integration | Full pipeline: init → serve/index → verify exclusions | Existing patterns |

All tests use `t.TempDir()` fixtures with synthetic `.gitignore` files and directory structures. No external services required.

## Risk Assessment

| Risk | Likelihood | Impact | Mitigation |
|------|-----------|--------|------------|
| Pattern matching edge cases (glob syntax) | Medium | Low | Comprehensive test matrix; minimal parser covers known patterns; full pathspec deferred |
| Breaking existing walker behavior | Low | High | All walkers preserve hidden-dir skip as baseline; no-`.gitignore` path is unchanged |
| Performance regression from Matcher construction | Low | Low | Matcher built once per walk; `.gitignore` is typically < 50 lines |
| `sources.yaml` backward compatibility | Low | Medium | New fields are optional with backward-compatible defaults (`ignore: []`, `recursive: true`) |

## Complexity Tracking

> No constitution violations. All four principles pass. No complexity justification needed.

## Post-Design Constitution Re-Check

### I. Composability First — PASS (confirmed)

The `ignore` package has zero dependencies beyond the Go standard library. Dewey remains independently installable. When no `.gitignore` exists and no `sources.yaml` ignore config is present, behavior is identical to today.

### II. Autonomous Collaboration — PASS (confirmed)

No MCP tool contract changes. The 40 existing tools continue to work identically. The ignore logic is internal to the walking layer.

### III. Observable Quality — PASS (confirmed)

Malformed `.gitignore` lines produce logged warnings with line numbers. `dewey doctor` verbose mode reports ignored directory counts. The system is auditable.

### IV. Testability — PASS (confirmed)

Every new component is testable in isolation:
- `ignore.Matcher` — pure function tests with string inputs
- `DiskSource` with ignore — `t.TempDir()` fixtures
- Vault walkers — `t.TempDir()` fixtures with `.gitignore`
- CLI commands — existing test patterns (`TestInitCmd_*`, `TestDoctorCmd_*`)

Coverage ratchets enforced by CI. Missing coverage is CRITICAL per constitution v1.2.0.

**Post-Design Gate Result: ALL PASS.**
