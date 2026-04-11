## Why

Disk and code sources with relative paths (e.g., `../gaze`, `../website`) show 0 pages because the relative path is passed directly to `filepath.Walk` without resolving against the vault root. When dewey's working directory differs from the vault root (which happens when OpenCode spawns the process), the path resolves to the wrong location or a nonexistent directory.

This affects all cross-repo source indexing — the primary value proposition of multi-repo Dewey. Only `path: "."` sources work reliably because they're special-cased to use the absolute vault path.

Fixes GitHub issue #40.

## What Changes

In `source/manager.go`, resolve all non-absolute source paths against `basePath` (the vault root) — not just `"."`. A 3-line fix in both `createDiskSource()` and `createCodeSource()`.

## Capabilities

### Modified Capabilities
- `createDiskSource()`: Resolves relative paths against vault basePath using `filepath.Join`
- `createCodeSource()`: Same fix applied

## Impact

- **source/manager.go**: 2 functions modified (~3 lines each)
- **source/manager_test.go**: New tests for relative path resolution
- No API changes, no schema changes, no new dependencies

## Constitution Alignment

### I. Autonomous Collaboration
**Assessment**: PASS — MCP tool interface unchanged.

### II. Composability First
**Assessment**: PASS — No new dependencies. Path resolution uses stdlib `filepath.Join`.

### III. Observable Quality
**Assessment**: PASS — Previously silent failure (0 pages) becomes correct indexing. Structured diagnostics log the resolved path.

### IV. Testability
**Assessment**: PASS — Testable with temp directories and relative path fixtures.
