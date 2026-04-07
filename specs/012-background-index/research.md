# Phase 0 Research: Background Index

**Branch**: `012-background-index` | **Date**: 2026-04-07

## R1: Current Startup Sequence Analysis

The current `executeServe()` in `main.go` (line 214) follows a strictly sequential startup:

1. **Backend resolution** — `resolveBackendType()`, instant
2. **Auto file logging** — setup if `.uf/dewey/` exists, ~1ms
3. **`initObsidianBackend()`** — the monolithic function (line 452):
   a. `resolveVaultPath()` — instant
   b. Store open via `store.New()` — ~7ms (SQLite WAL mode)
   c. Ollama check via `ensureOllama()` — ~5ms if running, up to 30s if auto-starting
   d. `vault.New()` — instant (builds in-memory structures)
   e. **`indexVault(vc)`** — **~30s on 255+ page repos** (BLOCKS)
   f. `LoadExternalPages()` — ~83ms
   g. `vc.Watch()` — ~28ms
4. **`newServer(b, readOnly, srvOpts...)`** — creates MCP server, instant
5. **`runServer(ctx, srv, httpAddr)`** — starts stdio/HTTP transport

The critical bottleneck is step 3e: `indexVault()` runs `IncrementalIndex()` which walks the filesystem, compares content hashes, re-parses changed files, persists blocks/links, and generates embeddings. On a 255-page repo, this takes ~32 seconds — exceeding OpenCode's 30-second MCP server startup timeout.

**Key insight**: The MCP server (`runServer`) is created and started AFTER all indexing completes. The fix is to reverse this: start the MCP server first, then index in the background.

## R2: MCP Server Lifecycle

The `newServer()` function (server.go line 48) creates the `mcp.Server` and registers all tools. It requires:
- `b backend.Backend` — the vault client (already created before indexing)
- `readOnly bool`
- `srvOpts ...serverOption` — embedder, store, vault path

**Critical finding**: `newServer()` does NOT require the vault to be indexed. It only needs the `vault.Client` instance (which is created by `vault.New()` before indexing). The tools read from the client's in-memory maps, which are empty until `indexVault()` populates them.

**Implication**: We can create the vault client, create the MCP server, start the transport, and THEN index the vault in a background goroutine. During the indexing window, tools will read from the persistent store (SQLite) which has the previous session's data.

## R3: SQLite Concurrent Access (WAL Mode)

The store uses SQLite with WAL (Write-Ahead Logging) mode, configured in `store/store.go`. WAL mode supports:
- **Multiple concurrent readers** — MCP tool handlers reading pages/blocks/embeddings
- **One writer at a time** — the background indexing goroutine writing new/changed pages

This is exactly the concurrency model we need. MCP tool handlers can read from the store while background indexing writes to it. Readers see a consistent snapshot (the state at the time their read transaction started).

## R4: Spec 011 Indexing Mutex

The `tools/indexing.go` `Indexing` struct (line 53) has a `sync.Mutex` field `mu` that guards the `Index()` and `Reindex()` MCP tool handlers. Both use `ix.mu.TryLock()` for non-blocking mutual exclusion — if the lock is held, they return "indexing operation already in progress" immediately.

**Design challenge**: Background startup indexing must acquire this same mutex so that:
1. If an agent calls `index` or `reindex` during background indexing, it gets the "already in progress" error
2. After background indexing completes, `index` and `reindex` work normally

**Options considered**:
1. **Share the `Indexing.mu` pointer** — Pass `&ix.mu` from `newServer()` back to `executeServe()`. Problem: `newServer()` creates the `Indexing` struct internally; the mutex is not exposed.
2. **External mutex injection** — Create the mutex in `executeServe()`, pass it to `NewIndexing()` via a new parameter or option. The `Indexing` struct uses this external mutex instead of its own.
3. **Package-level mutex** — Use a package-level `sync.Mutex` in `tools/`. Problem: violates "no global state" convention.

**Selected approach**: Option 2 — external mutex injection. Add a `sync.Mutex` pointer parameter to `NewIndexing()` (or a `WithMutex` option). The mutex is created in `executeServe()`, passed to both `NewIndexing()` and the background goroutine. This follows Dependency Injection (SOLID-D) and keeps the mutex lifecycle explicit.

## R5: `initObsidianBackend()` Decomposition

Currently `initObsidianBackend()` is a monolithic function that does everything: vault path resolution, store open, Ollama check, vault creation, indexing, external page loading, and watcher startup. It returns `(backend.Backend, []serverOption, func(), error)`.

For background indexing, we need to split this into two phases:
1. **Pre-server phase** (fast, <1s): vault path, store open, Ollama check, vault creation → returns backend + server opts + cleanup
2. **Background phase** (slow, ~30s): indexVault, LoadExternalPages, Watch → runs in goroutine

The cleanest approach is to extract the indexing/watcher portion into a separate function that `executeServe()` calls in a goroutine after starting the server.

## R6: `atomic.Bool` vs RWMutex for Index Readiness

Two approaches for signaling that background indexing is complete:

1. **`sync/atomic.Bool`** — Simple flag: `indexReady.Store(false)` at start, `indexReady.Store(true)` when done. Tools check `indexReady.Load()` to decide whether to use in-memory index or store fallback.
2. **`sync.RWMutex`** — Hold write lock during indexing, tools acquire read lock. Problem: this blocks ALL tool calls during indexing, which defeats the purpose.

**Selected approach**: `atomic.Bool`. It's simpler, non-blocking, and exactly matches the semantics: "is the in-memory index ready?" Tools don't need to wait for indexing — they just need to know whether to use the in-memory path or the store path.

## R7: Tool Behavior During Indexing

MCP tools in `tools/` read from `backend.Backend` (which is `*vault.Client`). The vault client's methods (`GetPage`, `SearchBlocks`, etc.) read from in-memory maps protected by `sync.RWMutex`.

**During background indexing** (before `indexReady` is true):
- In-memory maps are empty → tools return empty results
- BUT the persistent store has the previous session's data

**After background indexing** (after `indexReady` is true):
- In-memory maps are populated → tools return current results

**Design decision**: Rather than modifying every tool to check `indexReady` and fall back to the store, we can modify the `vault.Client` methods themselves to check the flag and delegate to the store when the in-memory index isn't ready. This keeps the change localized to the vault package.

**However**, this is a larger change than needed for the initial implementation. The simpler approach: since the persistent store already has the previous session's data, and `vault.Client` already has a `vaultStore` field, we can have the vault client's query methods check if pages are empty and fall back to the store. But this requires changes to many vault methods.

**Simplest viable approach**: Accept that during the ~30s indexing window, tools return empty results on a warm start (the store has data but the in-memory index is empty). This is acceptable because:
1. The server starts immediately (solving the timeout problem)
2. Most agent sessions don't query within the first 30 seconds
3. The `health` tool can report indexing status so agents know to wait

**Revised approach after further analysis**: The vault.Client already reads from the store for some operations. The key insight is that `indexVault()` populates BOTH the in-memory maps AND the store. On a warm start, the store already has data from the previous session. We need the vault client to serve queries from the store during the indexing window. This requires the `indexReady` flag to be checked in the vault client's query methods.

For the initial implementation, we'll use the `atomic.Bool` flag in `executeServe()` and pass it through to the server config. The `health` tool will report indexing status. Tools that need store fallback can be enhanced incrementally.
