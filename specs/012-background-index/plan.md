# Implementation Plan: Background Index

**Branch**: `012-background-index` | **Date**: 2026-04-07 | **Spec**: [spec.md](spec.md)
**Input**: Feature specification from `/specs/012-background-index/spec.md`

## Summary

Restructure `executeServe()` in `main.go` to start the MCP server before vault indexing. Today, vault indexing (~32s on 255+ page repos) blocks the MCP server from starting, exceeding OpenCode's 30-second timeout. The fix moves indexing to a background goroutine, achieving MCP server readiness in <1 second. During the ~30s indexing window, tools return results from the previous session's persistent store data. Background indexing shares a mutex with the `index`/`reindex` MCP tools (spec 011) to prevent concurrent indexing operations.

## Technical Context

**Language/Version**: Go 1.25 (per `go.mod`)
**Primary Dependencies**: `github.com/modelcontextprotocol/go-sdk/mcp` (MCP server), `github.com/spf13/cobra` (CLI), `github.com/charmbracelet/log` (logging), `sync/atomic` (readiness flag), `sync` (shared mutex)
**Storage**: SQLite via `modernc.org/sqlite` — WAL mode supports concurrent readers + one writer (critical for background indexing)
**Testing**: `go test -race -count=1 ./...` + Gaze quality gates (`--max-crapload=48`, `--max-gaze-crapload=40`)
**Target Platform**: darwin/linux x amd64/arm64 (cross-compiled via GoReleaser)
**Project Type**: CLI + MCP server
**Performance Goals**: MCP server ready in <2 seconds (down from ~32 seconds)
**Constraints**: No CGO, no external services required for core functionality, backward compatible with all 37 inherited MCP tools
**Scale/Scope**: ~50-line restructure of main.go, ~20-line change to server.go, ~15-line change to tools/indexing.go

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

### I. Composability First — ✅ PASS

Background indexing is internal to `executeServe()`. Dewey remains independently installable and usable. No new external dependencies. The change is invisible to MCP clients — they see the same tools with the same contracts. When the persistent store is not configured (in-memory mode), background indexing proceeds as before with no behavioral change.

### II. Autonomous Collaboration — ✅ PASS

All communication remains via MCP tool calls. The `health` tool gains an `indexing` field to report background indexing status — this is a backward-compatible addition to the structured JSON response. No runtime coupling, shared memory, or direct function calls with other tools.

### III. Observable Quality — ✅ PASS

Background indexing start and completion are logged with structured diagnostics (ISO 8601 timestamps, elapsed time, page counts). The `health` MCP tool reports indexing status so agents can assess result freshness. The `dewey status` CLI command is unchanged.

### IV. Testability — ✅ PASS

All new code is testable in isolation:
- The restructured `initObsidianBackend()` is tested without external services (existing pattern with `t.TempDir()` and `noEmbeddings=true`)
- The shared mutex is injected via dependency injection, testable with `TryLock()` assertions
- The `atomic.Bool` flag is testable by checking its value before and after background indexing
- Background goroutine behavior is testable by calling the extracted helper function directly
- All tests pass with `go test ./...` on a clean checkout

**Coverage strategy**: Contract-level tests for the restructured startup sequence, mutex sharing, and error handling. Existing `main_test.go` tests updated to reflect the new non-blocking `initObsidianBackend()`. New tests for `NewIndexing()` with external mutex parameter.

## Project Structure

### Documentation (this feature)

```text
specs/012-background-index/
├── plan.md              # This file
├── spec.md              # Feature specification
├── research.md          # Phase 0 research findings
├── quickstart.md        # Phase 1 architecture and design decisions
└── tasks.md             # Phase 2 task breakdown (created by /speckit.tasks)
```

### Source Code (files modified)

```text
main.go                  # Restructure executeServe(), split initObsidianBackend()
main_test.go             # Update startup tests, add background indexing tests
server.go                # Add WithIndexReady(), WithIndexMutex() options
tools/indexing.go        # Accept external *sync.Mutex in NewIndexing()
tools/indexing_test.go   # Test external mutex injection
```

**Structure Decision**: No new packages or files. This is a ~85-line restructure across 5 existing files. The change is surgical — it reorders the startup sequence and extracts the slow path into a background goroutine.

## Design Decisions

### D1: External Mutex Injection (SOLID-D: Dependency Inversion)

Create the `sync.Mutex` in `executeServe()` and inject it into `NewIndexing()` via a new `*sync.Mutex` parameter. The background goroutine locks this same mutex during startup indexing. When `mu` is nil, `NewIndexing()` creates an internal mutex (backward compatible for tests and non-serve usage).

**Alternative rejected**: Package-level mutex in `tools/` — violates "no global state" convention.
**Alternative rejected**: Exposing `Indexing.mu` via getter — breaks encapsulation, couples `executeServe()` to `Indexing` internals.

### D2: `atomic.Bool` for Index Readiness

Use `sync/atomic.Bool` to signal when background indexing completes. Starts `false`, set to `true` when done. Passed to `serverConfig` via `WithIndexReady()` so the `health` tool can report status.

**Alternative rejected**: `sync.RWMutex` — blocks tool calls during indexing, defeating the purpose.
**Alternative rejected**: Channel-based signaling — more complex, `atomic.Bool` is simpler for a boolean state.

### D3: `initObsidianBackend()` Split

Remove `indexVault()`, `LoadExternalPages()`, and `vc.Watch()` from `initObsidianBackend()`. These operations move to the background goroutine in `executeServe()`. The function signature is unchanged — it still returns `(backend.Backend, []serverOption, func(), error)` — but it completes in <1 second instead of ~32 seconds.

The vault client (`*vault.Client`) must be accessible in the background goroutine. Since `initObsidianBackend()` returns `backend.Backend` (an interface), the goroutine needs the concrete `*vault.Client`. Solution: type-assert `b.(*vault.Client)` in `executeServe()`, which is safe because we just created it.

### D4: Tool Behavior During Indexing Window

During the ~30s indexing window, the in-memory index is empty. Tools reading from `vault.Client` return empty results for in-memory-only operations.

**For v1**: Accept this limitation. The primary goal is "server starts within 2 seconds." The `health` tool reports indexing status. Semantic search tools already read from the persistent store (SQLite) and work immediately.

**Future enhancement** (separate spec): Add store-fallback to `vault.Client` query methods so navigation/search tools return previous session data during indexing.

### D5: Error Handling in Background Goroutine

If `indexVault()` fails: log the error, set `indexReady` to true (to unblock status checks), continue. The server operates with empty in-memory index. Developer retries via `index` MCP tool.

If `vc.Watch()` fails: log the error, continue. File watching is a convenience.

No panics, no restarts, no automatic retries. Per FR-008.

### D6: Logging (Spec 009 Structured Diagnostics)

```
INFO server ready transport=stdio tools=44 startup=847ms
INFO background indexing started
INFO incremental index complete new=3 changed=1 deleted=0 unchanged=251
INFO external pages loaded into vault count=12
INFO file watcher started elapsed=28ms
INFO background indexing complete elapsed=31.2s
```

### D7: Cleanup Function Adjustment

The cleanup function returned by `initObsidianBackend()` currently closes both the vault client and the persistent store. With background indexing, the cleanup must still work correctly:
- `vc.Close()` stops the watcher (if started) and releases resources
- `persistentStore.Close()` closes the SQLite connection

If the process exits during background indexing (e.g., OpenCode closes), the goroutine is terminated by the OS. Partially-indexed content already persisted to SQLite remains available in the next session. This is safe because SQLite WAL mode handles incomplete transactions.

## Complexity Tracking

> No constitution violations. All changes align with the four principles.

| Aspect | Complexity | Justification |
|--------|-----------|---------------|
| Shared mutex | Low | Standard Go pattern, injected via DI |
| `atomic.Bool` | Low | Single flag, no complex state machine |
| Background goroutine | Low | Fire-and-forget with error logging |
| `initObsidianBackend()` split | Low | Removing code, not adding |
| Total LOC change | ~85 lines | Restructure, not new feature |
