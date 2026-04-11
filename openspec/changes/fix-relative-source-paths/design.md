## Context

`createDiskSource()` and `createCodeSource()` in `source/manager.go` only resolve `path == "."` to the absolute vault path. Any other relative path (e.g., `"../gaze"`) is passed to `filepath.Walk` as-is, which resolves against the process CWD — not the vault root.

## Goals / Non-Goals

### Goals
- Resolve all relative source paths against the vault basePath
- Fix both disk and code source creation functions
- Add tests verifying relative path resolution

### Non-Goals
- Changing how basePath is determined (that's the caller's responsibility)
- Adding path validation (e.g., checking the directory exists)
- Modifying the sources.yaml format

## Decisions

**D1: Use `filepath.Join(basePath, path)` for relative paths.** `filepath.Join` handles `../` correctly — `filepath.Join("/a/b/c", "../gaze")` produces `/a/b/gaze`. No need for `filepath.Abs` since `basePath` is already absolute (guaranteed by `resolveVaultPath` in main.go).

**D2: Keep the `path == "."` special case.** It's equivalent to the new code (`filepath.Join(basePath, ".")` == `basePath`) but the explicit check makes the intent clear and preserves backward compatibility in test assertions.

**D3: Log the resolved path at DEBUG level.** Add `logger.Debug("resolved source path", "source", cfg.ID, "raw", rawPath, "resolved", path)` so path resolution issues are visible in verbose mode.

## Risks / Trade-offs

**Risk: None identified.** `filepath.Join` with an absolute basePath and a relative path is a well-defined operation. The only edge case is an absolute path in the config (e.g., `path: "/opt/data"`) — `filepath.Join` preserves it, which is correct.
