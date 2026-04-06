## Context

Dewey hardcodes `.dewey` as the workspace directory name in ~146 locations across 11 Go files. The ecosystem is standardizing on `.uf/<tool>/` as the per-tool workspace namespace.

## Goals / Non-Goals

### Goals
- Replace `.dewey` with `.uf/dewey` in all path construction
- Introduce a constant to centralize the workspace directory name
- Update `.gitignore` patterns in `dewey init`
- Rename lock file from `.dewey.lock` to `dewey.lock` (no longer needs hidden prefix)

### Non-Goals
- Renaming the `--vault` flag (separate concern)
- Backward compatibility with `.dewey/` (clean cut per issue #33)
- Migrating existing `.dewey/` data (users re-init)

## Decisions

**D1: Package-level constant.** Add `const deweyWorkspaceDir = ".uf/dewey"` to `main.go`. All path construction uses `filepath.Join(vaultPath, deweyWorkspaceDir)` instead of hardcoding `".dewey"`. CLI commands and tests reference this constant or construct paths the same way.

**D2: Lock file renamed.** `.dewey/.dewey.lock` → `.uf/dewey/dewey.lock`. The leading dot was needed when the lock file was in the repo root's `.dewey/` directory (hiding it from casual `ls`). Inside `.uf/dewey/` it's already hidden by the parent directory. Dropping the dot prefix makes it cleaner.

**D3: `os.MkdirAll` for parent creation.** `dewey init` uses `os.MkdirAll(filepath.Join(vaultPath, ".uf", "dewey"), 0o755)` which creates both `.uf/` and `.uf/dewey/` in one call. No dependency on `uf init` having run first.

**D4: `.gitignore` patterns.** The granular patterns from issue #23 are updated:
```
.uf/dewey/graph.db
.uf/dewey/graph.db-shm
.uf/dewey/graph.db-wal
.uf/dewey/dewey.log
.uf/dewey/dewey.lock
```

**D5: Constitution path.** The constitution references `.dewey/` in two places. Both updated to `.uf/dewey/`.

## Risks / Trade-offs

**Risk: Stale `.dewey/` directories.** Users upgrading from older versions will have orphaned `.dewey/` directories. The clean-cut approach means dewey won't warn about them. Mitigation: release notes will document the migration step (`rm -rf .dewey && dewey init`).

**Risk: Lock file name change.** `detectLockHolder()` reads the lock file by name. The name change from `.dewey.lock` to `dewey.lock` means the doctor command must use the new name. Since there's no backward compat, this is just part of the rename.
