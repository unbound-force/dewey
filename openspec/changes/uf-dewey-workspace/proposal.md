## Why

The Unbound Force ecosystem is consolidating all per-repo tool directories under a unified `.uf/` namespace (org Spec 025). Dewey's workspace directory moves from `.dewey/` to `.uf/dewey/`. This is a clean cut — no backward compatibility with `.dewey/`.

Fixes GitHub issue #33.

## What Changes

Replace every reference to `.dewey/` with `.uf/dewey/` across the codebase. Introduce a package-level constant for the workspace directory name to prevent future hardcoding. Update `dewey init` to create `.uf/dewey/` (with `os.MkdirAll` to create the `.uf/` parent). Update `.gitignore` patterns from `.dewey/*` to `.uf/dewey/*`.

The `--vault` flag is unchanged — it points to the project root (CWD by default), and dewey derives `.uf/dewey/` internally.

## Capabilities

### Modified Capabilities
- `dewey init`: Creates `.uf/dewey/` instead of `.dewey/`. Creates `.uf/` parent if needed.
- `dewey serve`: Reads workspace from `.uf/dewey/` (store, config, sources, log, lock)
- `dewey index`/`reindex`/`status`/`doctor`/`manifest`: All use `.uf/dewey/` workspace path
- `.gitignore` patterns: Updated from `.dewey/graph.db` etc. to `.uf/dewey/graph.db` etc.

### Removed Capabilities
- `.dewey/` directory support: No backward compatibility, no fallback, no migration

## Impact

- **11 Go source files**: 146 references to `.dewey` replaced with `.uf/dewey`
- **3 documentation files**: AGENTS.md, README.md, constitution.md updated
- **opencode.json**: No change needed (`--vault .` points to project root, not workspace)
- **Users upgrading**: Must `rm -rf .dewey/` and run `dewey init` (or `uf init`)
- **Lock file**: Renamed from `.dewey/.dewey.lock` to `.uf/dewey/dewey.lock` (drop dot prefix since already inside hidden directory)

## Constitution Alignment

Assessed against the Unbound Force org constitution.

### I. Autonomous Collaboration

**Assessment**: PASS

MCP tool interface unchanged. Only internal workspace paths change — no impact on artifact-based communication.

### II. Composability First

**Assessment**: PASS

Dewey remains independently installable. The `.uf/` parent directory is created by `dewey init` via `os.MkdirAll` — no dependency on `uf init` having run first.

### III. Observable Quality

**Assessment**: PASS

All diagnostic paths (doctor, status, log file) work identically with the new directory. Provenance metadata and index auditability unchanged.

### IV. Testability

**Assessment**: PASS

All tests use `t.TempDir()` fixtures with programmatic path construction. Changing the constant updates all test paths automatically.
