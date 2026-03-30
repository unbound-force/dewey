# Research: Unified Ignore Support

**Branch**: `006-unified-ignore` | **Date**: 2026-03-30

## R1: Walker Inventory

All four filesystem walkers use `filepath.Walk` with identical hidden-directory skipping logic. Each is a separate call site that must be updated:

| Walker | Location | Purpose | Current Skip Logic |
|--------|----------|---------|-------------------|
| `Load()` | `vault/vault.go:141` | Initial vault walk on `dewey serve` | `strings.HasPrefix(info.Name(), ".")` → `filepath.SkipDir` |
| `addWatcherDirs()` | `vault/vault.go:302` | Adds directories to fsnotify watcher | Same, plus `path != root` guard |
| `walkVault()` | `vault/vault_store.go:224` | Incremental index walk (hash comparison) | Same as `Load()` |
| `DiskSource.List()` | `source/disk.go:55` | External source directory scan | Same as `Load()` |

Additionally, `walkDiskFiles()` at `source/disk.go:140` is used by `DiskSource.Diff()` and has the same pattern.

**Finding**: All five walk functions have identical skip logic (3 lines each). This is the duplication that FR-012 requires us to eliminate.

## R2: Event Handler Analysis

The file watcher event handler at `vault/vault.go:337-362` (`handleEvent()`) performs two checks:
1. Skip non-`.md` files (line 339)
2. Skip hidden directories via `strings.Contains(event.Name, "/.")` (line 344)

The hidden-directory check uses string containment on the full path, not `filepath.Walk`'s callback pattern. The ignore matcher must provide a method that works with full paths (checking if any path component matches an ignore pattern), not just directory names.

**Finding**: `handleEvent()` needs a `MatchPath(relPath string) bool` method that checks each path component against ignore patterns.

## R3: Source Config Structure

`SourceConfig` at `source/config.go:14-20`:
```go
type SourceConfig struct {
    ID              string         `yaml:"id"`
    Type            string         `yaml:"type"`
    Name            string         `yaml:"name"`
    Config          map[string]any `yaml:"config"`
    RefreshInterval string         `yaml:"refresh_interval,omitempty"`
}
```

The `Config map[string]any` field holds type-specific configuration. For disk sources, it currently contains only `path` (string). The new `ignore` (list of strings) and `recursive` (bool) fields will be read from this map.

**Finding**: No struct changes needed. `createDiskSource()` at `source/manager.go:75-83` already reads `path` from the map; we add `ignore` and `recursive` reads in the same pattern.

## R4: Vault-to-Source Config Bridge

The vault's `WithStore()` option (vault/vault.go:110-116) hardcodes `sourceID: "disk-local"`. The vault does not currently read any config from `sources.yaml` — it only uses the sourceID for store operations.

To read ignore patterns from the `disk-local` source entry, the vault needs access to the source config. Options:

1. **Pass ignore patterns via a new `WithIgnorePatterns()` option** — the caller (main.go) reads `sources.yaml` and passes patterns to the vault
2. **Have the vault read `sources.yaml` directly** — creates a dependency from `vault` → `source`
3. **Pass the full `SourceConfig` via a new option** — over-exposes config internals

**Decision**: Option 1. The vault receives ignore patterns as `[]string` via a new `WithIgnorePatterns(patterns []string) Option`. The caller (`main.go:initObsidianBackend()`) reads the `disk-local` source config and extracts the ignore patterns. This keeps the vault package independent of the source package.

## R5: .gitignore Pattern Syntax

Real `.gitignore` files in Unbound Force repos contain these pattern types:

```gitignore
# Comments (skip)
node_modules/       # Directory pattern (trailing /)
dist/               # Directory pattern
*.log               # File glob
!important.md       # Negation
.dewey/             # Hidden dir (already skipped, but harmless to also match)
                    # Blank lines (skip)
```

The minimal parser needs to handle:
1. **Blank lines** — skip
2. **Comments** (`#` prefix) — skip
3. **Negation** (`!` prefix) — mark pattern as negated
4. **Directory patterns** (trailing `/`) — match directory names only
5. **File globs** (`*`, `?`) — use `filepath.Match()` for glob evaluation
6. **Plain names** — exact match against directory or file names

**Not needed** (deferred):
- `**/` double-star (recursive directory matching) — not found in our repos
- Leading `/` (root-relative patterns) — not found in our repos
- Escape characters (`\#`, `\ `) — not found in our repos

**Finding**: `filepath.Match()` from the Go standard library handles `*`, `?`, `[...]` glob syntax. We use it for all pattern matching, which covers both exact names and globs.

## R6: DiskSource Constructor Extension

Current `NewDiskSource()` signature:
```go
func NewDiskSource(id, name, basePath string) *DiskSource
```

Needs to become:
```go
func NewDiskSource(id, name, basePath string, opts ...DiskSourceOption) *DiskSource
```

Using the options pattern (consistent with `vault.New()`) to add ignore patterns and recursive flag without breaking the existing signature. The `DiskSourceOption` type follows the same `func(*DiskSource)` pattern as `vault.Option`.

**Finding**: Options pattern preserves backward compatibility. Existing callers pass no options and get identical behavior.

## R7: dewey init Parent Directory Detection

The `newInitCmd()` at `cli.go:204` generates a hardcoded `sources.yaml` template. It does not currently detect parent-directory sources because it only creates a single `disk-local` source with `path: "."`.

FR-007 says `dewey init` should default `recursive: false` for sources with `path: "../"`. Since `dewey init` only creates the `disk-local` source (not external sources), this requirement applies when a user later runs `dewey source add` with a parent path. However, the spec says "dewey init MUST generate" — this means the init template itself should document the pattern, and the `source add` command should apply the default.

**Finding**: The `dewey source add` command (cli.go) is the right place to apply `recursive: false` for parent paths. The `dewey init` template can include a comment showing the pattern. Both should be updated.

## R8: dewey doctor Verbose Mode

The `runDoctorChecks()` function at `cli.go:1169` does not currently have a verbose flag. The doctor command is invoked via `newDoctorCmd()` which has no `--verbose` flag.

FR-013 says doctor should report ignored directory counts in verbose mode. This requires:
1. Adding a `--verbose` flag to the doctor command
2. When verbose, building a `Matcher` for the vault path and counting how many directories would be skipped
3. Reporting the count in the Workspace section

**Finding**: The verbose doctor check is a read-only diagnostic — it walks the vault directory, builds a Matcher, and counts ignored directories without modifying any state.

## R9: Backward Compatibility Verification

The existing behavior when no `.gitignore` exists:
- All walkers skip hidden directories (names starting with `.`)
- All non-hidden directories are traversed
- All `.md` files are indexed

With the new `Matcher`:
- If no `.gitignore` exists at the walk root → `Matcher` has zero gitignore patterns
- If no `ignore` patterns in `sources.yaml` → `Matcher` has zero extra patterns
- Hidden-directory skipping is preserved as a baseline (hardcoded in `Matcher`, not dependent on patterns)
- Result: identical behavior to today

**Finding**: Backward compatibility is guaranteed by construction. The `Matcher` with empty pattern lists produces the same results as the current hardcoded checks.

## Open Questions — RESOLVED

All questions from the exploration session have been resolved:

| # | Question | Resolution |
|---|----------|------------|
| 1 | How does the vault get ignore patterns from sources.yaml? | Via `WithIgnorePatterns()` option; caller reads config (R4) |
| 2 | Should we use a third-party gitignore library? | No — minimal parser covers all needed patterns (R5, D2) |
| 3 | Should nested .gitignore files be processed? | No — root-level only per FR-002 (D3) |
| 4 | How does handleEvent() check ignore patterns? | Via `MatchPath()` method that checks path components (R2) |
| 5 | How does DiskSource get ignore/recursive config? | Via options pattern on `NewDiskSource()` (R6) |
| 6 | Where does `recursive: false` default apply? | `dewey init` template + `dewey source add` for parent paths (R7) |
| 7 | How does doctor report ignored counts? | New `--verbose` flag, walks vault and counts (R8) |

**NEEDS CLARIFICATION: None. All questions resolved.**
