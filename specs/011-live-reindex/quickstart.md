# Quickstart: Live Reindex

**Branch**: `011-live-reindex` | **Date**: 2026-04-07

## What This Feature Does

Adds two MCP tools (`index` and `reindex`) that allow AI agents to trigger source re-indexing while `dewey serve` is running. Also adds two OpenCode slash commands (`/dewey-index` and `/dewey-reindex`) as human-friendly entry points.

## Architecture at a Glance

```
Agent/Developer
    │
    ├─ MCP tool call: "index" ──────┐
    │                                │
    ├─ MCP tool call: "reindex" ────┤
    │                                │
    ├─ /dewey-index (slash cmd) ────┤  (instructs agent to call MCP tool)
    │                                │
    └─ /dewey-reindex (slash cmd) ──┘
                                     │
                                     ▼
                            tools/indexing.go
                            ┌─────────────────┐
                            │   Indexing       │
                            │   ├─ mu (Mutex)  │
                            │   ├─ store       │
                            │   ├─ embedder    │
                            │   └─ vaultPath   │
                            └────────┬────────┘
                                     │
                    ┌────────────────┼────────────────┐
                    ▼                ▼                ▼
            source.Manager    vault.Persist*    embed.Embedder
            (fetch docs)      (parse & store)   (generate vectors)
                    │                │                │
                    └────────────────┼────────────────┘
                                     ▼
                              store.Store
                            (SQLite graph.db)
```

## Key Design Decisions

1. **Indexing struct pattern**: Follows `Learning` struct — dependency injection via constructor, MCP handler methods. Store, embedder, and vault path injected.

2. **Mutex for mutual exclusion**: `sync.Mutex.TryLock()` provides non-blocking concurrency control. Second caller gets immediate error, not a queue.

3. **Reindex safety**: Deletes pages per-source via `store.DeletePagesBySource()`, skipping `disk-local` and `learning` sources. Does NOT delete `graph.db` (server is using it).

4. **Pipeline reimplementation**: The indexing orchestration (~50 lines) is reimplemented in `tools/indexing.go` calling the same shared functions (`vault.ParseDocument`, `vault.PersistBlocks`, `vault.PersistLinks`, `vault.GenerateEmbeddings`). This avoids cross-package coupling with `package main`.

5. **Server wiring**: New `WithVaultPath()` server option. `registerIndexingTools()` follows `registerLearningTools()` pattern. Write-only (excluded in read-only mode).

## Files to Create

| File | Purpose |
|------|---------|
| `tools/indexing.go` | `Indexing` struct with `Index` and `Reindex` MCP handlers |
| `tools/indexing_test.go` | Tests: happy path, error paths, mutual exclusion, reindex safety |
| `.opencode/command/dewey-index.md` | Slash command instructing agent to call `index` tool |
| `.opencode/command/dewey-reindex.md` | Slash command instructing agent to call `reindex` tool |

## Files to Modify

| File | Change |
|------|--------|
| `server.go` | Add `vaultPath` to `serverConfig`, `WithVaultPath()` option, `registerIndexingTools()`, wire in `newServer()` |
| `server_test.go` | Update tool count ranges (+2 for indexing tools), add registration tests |
| `main.go` | Pass `WithVaultPath(vp)` in `initObsidianBackend()` |
| `types/tools.go` | Add `IndexInput` and `ReindexInput` types |

## How to Test

```bash
# Run all tests
go test -race -count=1 ./...

# Run only indexing tool tests
go test -race -count=1 ./tools/ -run TestIndexing

# Run server registration tests
go test -race -count=1 . -run TestNewServer
```

## Response Format

Both tools return structured JSON:

```json
{
  "status": "completed",
  "sources_processed": 2,
  "pages_indexed": 15,
  "embeddings_generated": 15,
  "embeddings_skipped": 0,
  "elapsed_ms": 1250,
  "sources": [
    {"id": "github-org", "type": "github", "documents": 10, "error": ""},
    {"id": "web-docs", "type": "web", "documents": 5, "error": ""}
  ]
}
```
