# Phase 1 Quickstart: Background Index

**Branch**: `012-background-index` | **Date**: 2026-04-07

## Architecture Overview

This feature restructures the `executeServe()` startup sequence so the MCP server starts before vault indexing. The change is localized to `main.go`, `server.go`, and their tests — no new packages are needed.

### Before (Current)

```
executeServe()
  └─ initObsidianBackend()     ← monolithic, ~32s
       ├─ store.New()           ~7ms
       ├─ ensureOllama()        ~5ms
       ├─ vault.New()           instant
       ├─ indexVault()          ~30s  ← BLOCKS
       ├─ LoadExternalPages()   ~83ms
       └─ vc.Watch()            ~28ms
  └─ newServer()                instant
  └─ runServer()                ← MCP server starts HERE (after 32s)
```

### After (New)

```
executeServe()
  └─ initObsidianBackendFast()  ← fast path only, <1s
       ├─ store.New()            ~7ms
       ├─ ensureOllama()         ~5ms
       └─ vault.New()            instant
  └─ newServer()                 instant
  └─ log "server ready"          ← MCP server ready in <1s
  └─ go backgroundIndex()        ← goroutine
       ├─ indexMu.Lock()         acquire shared mutex
       ├─ indexVault()           ~30s (non-blocking)
       ├─ LoadExternalPages()    ~83ms
       ├─ vc.Watch()             ~28ms
       ├─ indexReady.Store(true) signal completion
       └─ indexMu.Unlock()       release mutex
  └─ runServer()                 ← already accepting calls
```

## Key Design Decisions

### D1: External Mutex Injection

The `tools/indexing.go` `Indexing` struct has a `sync.Mutex` for mutual exclusion between `index` and `reindex` MCP tools. Background startup indexing must participate in this same mutex.

**Decision**: Create the mutex in `executeServe()` and inject it into `NewIndexing()` via a new `*sync.Mutex` parameter. The background goroutine locks this same mutex during startup indexing.

**Alternative rejected**: Package-level mutex in `tools/` — violates "no global state" convention (AGENTS.md).

### D2: `atomic.Bool` for Index Readiness

**Decision**: Use `sync/atomic.Bool` (`indexReady`) to signal when background indexing completes. Starts `false`, set to `true` when the background goroutine finishes. Passed to `serverConfig` so the `health` tool can report indexing status.

**Alternative rejected**: `sync.RWMutex` — would block tool calls during indexing, defeating the purpose.

### D3: `initObsidianBackend()` Split

**Decision**: Split `initObsidianBackend()` into two functions:
1. `initObsidianBackend()` — fast path: vault path, store, Ollama, vault creation. Returns backend + opts + cleanup.
2. `backgroundIndex()` — slow path: indexVault, LoadExternalPages, Watch. Runs in goroutine.

The slow-path operations are extracted from `initObsidianBackend()` into inline code in `executeServe()` (or a helper function). This avoids changing the function signature of `initObsidianBackend()` beyond removing the indexing/watcher steps.

### D4: Tool Behavior During Indexing Window

During the ~30s indexing window, the in-memory index is empty. Tools reading from `vault.Client` will return empty results.

**Decision for v1**: Accept empty results during the indexing window. The `health` tool reports `indexing: true/false` so agents can check status. This is acceptable because:
- The server starts (solving the timeout problem — the primary goal)
- The persistent store has previous session data (agents can use `semantic_search` which reads from the store directly)
- Most agent sessions don't query within the first 30 seconds

**Future enhancement**: Add store-fallback to vault.Client query methods so tools return previous session data during indexing. This is a separate spec.

### D5: Error Handling in Background Goroutine

**Decision**: If `indexVault()` fails in the background goroutine, log the error and continue. The server remains operational with empty in-memory index. The developer can retry via the `index` MCP tool. No crash, no restart.

If `vc.Watch()` fails, log the error and continue. File watching is a convenience — the server works without it.

### D6: Logging

**Decision**: Use structured diagnostics per spec 009:
- `"background indexing started"` — immediately after goroutine launches
- `"background indexing complete"` — with elapsed time, page counts
- `"background indexing failed"` — with error, if indexVault fails

## Contracts

### Modified Function: `executeServe()`

```go
// executeServe contains the shared serve logic. Restructured for background
// indexing: the MCP server starts before vault indexing begins.
func executeServe(readOnly bool, backendType, vaultPath, dailyFolder, httpAddr string, noEmbeddings bool) error
```

No signature change. Internal restructuring only.

### Modified Function: `initObsidianBackend()`

```go
// initObsidianBackend initializes the Obsidian/vault backend (fast path).
// Vault indexing and file watcher startup are deferred to the caller
// for background execution.
func initObsidianBackend(vaultPath, dailyFolder string, noEmbeddings bool) (backend.Backend, []serverOption, func(), error)
```

Same signature, but no longer calls `indexVault()`, `LoadExternalPages()`, or `vc.Watch()`. These are moved to the background goroutine in `executeServe()`.

### Modified Function: `NewIndexing()`

```go
// NewIndexing creates a new Indexing tool handler. The mutex parameter
// enables shared mutual exclusion with background startup indexing.
// When mu is nil, an internal mutex is used (backward compatible).
func NewIndexing(s *store.Store, e embed.Embedder, vaultPath string, mu *sync.Mutex) *Indexing
```

New `mu *sync.Mutex` parameter. When non-nil, replaces the internal mutex.

### New Server Option: `WithIndexReady()`

```go
// WithIndexReady sets the atomic flag that tracks background indexing status.
func WithIndexReady(ready *atomic.Bool) serverOption
```

### New Server Option: `WithIndexMutex()`

```go
// WithIndexMutex sets the shared mutex for indexing mutual exclusion.
func WithIndexMutex(mu *sync.Mutex) serverOption
```

## Coverage Strategy

### What to Test

1. **`initObsidianBackend()` no longer blocks on indexing** — verify it returns quickly without calling `indexVault()`
2. **Background goroutine completes indexing** — verify `indexReady` transitions from false to true
3. **Mutex shared between background indexing and MCP tools** — verify `index` tool returns "already in progress" during background indexing
4. **Error handling in background goroutine** — verify server continues when indexVault fails
5. **`NewIndexing()` with external mutex** — verify backward compatibility when mu is nil

### How to Test

- **Unit tests in `main_test.go`**: Test the restructured `initObsidianBackend()` returns without indexing. Test the background indexing helper function directly.
- **Unit tests in `tools/indexing_test.go`**: Test `NewIndexing()` with external mutex. Test `TryLock` behavior when external mutex is held.
- **Integration test**: Start server, verify `health` tool reports indexing status, verify tools work after indexing completes.

### Coverage Targets

- All new code paths must have contract-level tests
- Existing tests must continue to pass (no regressions)
- CRAP score for modified functions must stay below 48 (CI threshold)
