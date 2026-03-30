# Quickstart: Unified Ignore Support

**Branch**: `006-unified-ignore` | **Date**: 2026-03-30

## What This Feature Does

Adds `.gitignore` respect and configurable ignore patterns to all dewey filesystem walkers. Before this feature, `dewey serve` on a project with `node_modules/` indexes hundreds of junk markdown files, causing 40+ second startup times. After this feature, dewey automatically skips gitignored directories, starting in under 5 seconds.

## Key Files to Create/Modify

### New Files

| File | Purpose |
|------|---------|
| `ignore/ignore.go` | `Matcher` type — parses `.gitignore` + extra patterns, evaluates matches |
| `ignore/ignore_test.go` | Contract tests for pattern parsing and matching |

### Modified Files

| File | Change |
|------|--------|
| `vault/vault.go` | `Load()`, `addWatcherDirs()`, `handleEvent()` use `Matcher`; new `WithIgnorePatterns()` option |
| `vault/vault_store.go` | `walkVault()` accepts and uses `Matcher` |
| `source/disk.go` | `DiskSource` gains ignore patterns + recursive flag; `List()` and `walkDiskFiles()` use `Matcher` |
| `source/manager.go` | `createDiskSource()` reads `ignore`/`recursive` from config map |
| `source/config.go` | `validateSourceConfig()` accepts `ignore`/`recursive` for disk sources |
| `cli.go` | `dewey init` generates `recursive: false` for parent paths; `dewey doctor` verbose reports ignored counts |
| `main.go` | `initObsidianBackend()` reads `disk-local` ignore config, passes to vault |

## Implementation Order

```
Phase 1: ignore package (no dependencies on existing code)
    ↓
Phase 2: source package (imports ignore, no vault changes)
    ↓
Phase 3: vault package (imports ignore, uses Matcher in walkers)
    ↓
Phase 4: CLI integration (init, doctor, main.go wiring)
    ↓
Phase 5: Integration tests (end-to-end verification)
```

## Quick Reference: Pattern Syntax

```gitignore
# Comment (ignored)
node_modules/    # Skip directory named "node_modules"
dist/            # Skip directory named "dist"
*.log            # Skip files matching glob *.log
!important.md    # Negation — do NOT skip this file
                 # Blank line (ignored)
```

## Quick Reference: sources.yaml Extension

```yaml
sources:
  - id: disk-local
    type: disk
    name: local
    config:
      path: "."
      ignore:           # NEW — additional patterns beyond .gitignore
        - drafts
        - wip
      recursive: true   # NEW — default true; set false for parent dirs

  - id: disk-org
    type: disk
    name: org
    config:
      path: "../"
      recursive: false  # Only index top-level .md files
```

## Testing Approach

All tests use `t.TempDir()` with synthetic directory structures:

```go
// Example test fixture
func setupTestVault(t *testing.T) string {
    dir := t.TempDir()
    // Create .gitignore
    os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("node_modules/\n"), 0o644)
    // Create directories
    os.MkdirAll(filepath.Join(dir, "node_modules", "pkg"), 0o755)
    os.MkdirAll(filepath.Join(dir, "docs"), 0o755)
    // Create .md files
    os.WriteFile(filepath.Join(dir, "node_modules", "pkg", "README.md"), []byte("# Junk"), 0o644)
    os.WriteFile(filepath.Join(dir, "docs", "guide.md"), []byte("# Guide"), 0o644)
    os.WriteFile(filepath.Join(dir, "README.md"), []byte("# Root"), 0o644)
    return dir
}
```

Run tests: `go test -race -count=1 ./ignore/... ./source/... ./vault/... ./...`

## Gotchas

1. **`walkVault()` is a package-level function** (not a method on `Client`). It needs to accept a `Matcher` parameter — this changes its signature, which means `VaultStore.IncrementalIndex()` (the caller) must also be updated.

2. **`handleEvent()` uses full paths**, not directory names. Use `ShouldSkipPath()` (checks each path component), not `ShouldSkip()` (checks a single name).

3. **`addWatcherDirs()` has a special case**: it skips hidden dirs only when `path != root` (the root itself might be hidden). The `Matcher` should not skip the root directory.

4. **`walkDiskFiles()` in `source/disk.go`** is used by `Diff()` and must also use the `Matcher` — otherwise `List()` and `Diff()` would disagree on which files exist.

5. **The `Config map[string]any`** stores YAML-parsed values. `ignore` will be `[]any` (not `[]string`) — you need to type-assert each element. Use the existing `extractStringList()` helper in `source/manager.go`.

6. **No data model changes** — this feature does not modify the SQLite schema, the `store` package, or any MCP tool contracts.
