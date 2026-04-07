# Implementation Plan: Live Reindex

**Branch**: `011-live-reindex` | **Date**: 2026-04-07 | **Spec**: [spec.md](spec.md)
**Input**: Feature specification from `/specs/011-live-reindex/spec.md`

## Summary

Add `index` and `reindex` MCP tools that allow AI agents and developers to trigger source re-indexing while `dewey serve` is running, without terminal access. The tools wrap the existing `indexDocuments()` pipeline from `cli.go`, protected by a `sync.Mutex` for mutual exclusion. Two OpenCode slash commands (`/dewey-index`, `/dewey-reindex`) provide human-friendly entry points that instruct the agent to call the corresponding MCP tools.

## Technical Context

**Language/Version**: Go 1.25 (per `go.mod`)
**Primary Dependencies**: `github.com/modelcontextprotocol/go-sdk/mcp` (MCP SDK), `github.com/spf13/cobra` (CLI), `github.com/charmbracelet/log` (logging), `github.com/unbound-force/dewey/source` (source manager), `github.com/unbound-force/dewey/store` (SQLite persistence), `github.com/unbound-force/dewey/embed` (Ollama embeddings), `github.com/unbound-force/dewey/vault` (document parsing/persistence)
**Storage**: SQLite via `modernc.org/sqlite` — single database `.uf/dewey/graph.db`
**Testing**: Standard library `testing` package, in-memory SQLite (`:memory:`), no external assertion libraries
**Target Platform**: macOS/Linux (darwin/linux × amd64/arm64)
**Project Type**: CLI + MCP server
**Performance Goals**: Round-trip from tool call to searchable content under 30 seconds for 100 files (SC-001)
**Constraints**: No CGO, no data leaving the developer's machine, server must remain responsive during indexing (FR-006)
**Scale/Scope**: 2 new MCP tools, 2 slash commands, 1 new file in `tools/`, modifications to `server.go` and `server_test.go`

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Status | Evidence |
|-----------|--------|----------|
| **I. Composability First** | ✅ PASS | The new MCP tools are self-contained within the Dewey server. They do not create dependencies on OpenCode, Swarm, Gaze, or any other ecosystem component. The slash commands are `.md` files that instruct agents — they work with any MCP client, not just OpenCode. When Ollama is unavailable, indexing proceeds without embeddings (graceful degradation per FR-007). |
| **II. Autonomous Collaboration** | ✅ PASS | The tools communicate exclusively through MCP tool calls with structured JSON responses. No runtime coupling, shared memory, or direct function calls. The `index` and `reindex` tools return structured summaries (sources processed, pages new/changed/deleted, embeddings generated, elapsed time) per FR-004. |
| **III. Observable Quality** | ✅ PASS | Both tools return structured summaries with provenance metadata: source counts, page counts, embedding counts, and elapsed time. Progress is logged using structured diagnostics (FR-011). The `health` tool already reports source freshness and embedding coverage — newly indexed content is immediately visible there. |
| **IV. Testability** | ✅ PASS | The `Indexing` struct follows the same dependency injection pattern as `Learning` — `store` and `embedder` are injected via constructor. Tests use in-memory SQLite (`:memory:`) and mock embedders. No external services required. The indexing pipeline is already tested in `cli.go` — the MCP tools are thin wrappers. Coverage strategy: test the tool handlers directly with mock dependencies, verifying input validation, error paths, mutual exclusion, and result structure. |

## Project Structure

### Documentation (this feature)

```text
specs/011-live-reindex/
├── plan.md              # This file
├── research.md          # Phase 0 output
├── quickstart.md        # Phase 1 output
└── tasks.md             # Phase 2 output (/speckit.tasks)
```

### Source Code (repository root)

```text
# Files to CREATE
tools/indexing.go              # Indexing struct, Index handler, Reindex handler
tools/indexing_test.go         # Tests for indexing tools
.opencode/command/dewey-index.md    # Slash command for /dewey-index
.opencode/command/dewey-reindex.md  # Slash command for /dewey-reindex

# Files to MODIFY
server.go                      # registerIndexingTools(), wire in newServer()
server_test.go                 # Update tool count expectations, add indexing tool tests
```

**Structure Decision**: This feature adds a single new file in the existing `tools/` package following the established pattern (one file per tool category: `learning.go`, `semantic.go`, `navigate.go`, etc.). The `Indexing` struct mirrors the `Learning` struct pattern — dependency injection via constructor, MCP handler methods, structured JSON responses. No new packages or architectural changes.

## Phase 0: Research

### R1: Existing Indexing Pipeline

The `indexDocuments()` function in `cli.go` (line 947) is the core indexing pipeline. It:
1. Iterates over `allDocs` (map of source ID → documents)
2. For each document: parses content via `vault.ParseDocument()`, upserts page records, persists blocks via `vault.PersistBlocks()`, persists links via `vault.PersistLinks()`, generates embeddings via `vault.GenerateEmbeddings()`
3. Updates source records in the store (`InsertSource`/`UpdateLastFetched`/`UpdateSourceStatus`)
4. Returns total indexed count

This function is already well-factored — it takes `*store.Store`, `map[string][]source.Document`, `[]source.SourceConfig`, and `embed.Embedder` as parameters. The MCP tools can call it directly.

### R2: Source Manager Lifecycle

`source.NewManager(configs, basePath, cacheDir)` creates a manager from source configs. `mgr.FetchAll(sourceName, force, lastFetchedTimes)` fetches all (or filtered) sources and returns `(*FetchResult, map[string][]Document)`. The MCP tools need:
- `sourcesDir` (path to `.uf/dewey/`) to find `sources.yaml`
- `vaultPath` (base path) for resolving relative source paths
- `store` for persistence and last-fetched times
- `embedder` for embedding generation

### R3: In-Process DB Access

When `dewey serve` runs, it holds `*store.Store` open for the lifetime of the process. The MCP tools run in the same process — they have direct access to the store via the `Indexing` struct. No lock conflict. SQLite WAL mode allows concurrent reads while a write is in progress, so other MCP tools (search, get_page, traverse) remain functional during indexing (FR-006).

### R4: Mutual Exclusion Pattern

A `sync.Mutex` on the `Indexing` struct prevents concurrent index/reindex operations. The handler acquires the lock with `TryLock()` — if it fails, the tool returns an error result indicating an operation is already in progress (FR-005). This is simpler and more correct than an atomic flag because it also provides memory ordering guarantees.

### R5: Reindex Safety

The `reindex` tool must NOT delete `disk-local` or `learning` source pages (FR-009). The existing `store.DeletePagesBySource(sourceID)` deletes pages for a specific source ID. The reindex handler:
1. Loads source configs from `sources.yaml`
2. For each config where `cfg.Type != "disk"` (i.e., not the local vault source), calls `store.DeletePagesBySource(cfg.ID)`
3. Also skips any source with ID `"learning"` (agent learnings)
4. Then runs the full indexing pipeline with `force=true`

Wait — re-reading the spec more carefully: FR-009 says "The `reindex` tool MUST NOT affect local vault content (pages indexed by `IncrementalIndex` at serve startup) — only external source content is rebuilt." The `disk-local` source IS the local vault content. So reindex should skip `disk-local` AND `learning` sources when deleting, but should still re-fetch and re-index all OTHER configured sources.

Actually, looking at the CLI `reindex` command (line 700), it deletes the entire `graph.db` and rebuilds from scratch. But the MCP `reindex` tool can't do that — the server is using the database. Instead, it does a per-source delete-and-rebuild for external sources only. This is a different (safer) approach than the CLI command.

### R6: Vault Path Access

The `Indexing` struct needs the vault path to:
1. Find `.uf/dewey/sources.yaml` for source config
2. Pass as `basePath` to `source.NewManager()` for resolving relative paths
3. Find the cache directory (`.uf/dewey/cache/`)

The vault path is available in `executeServe()` via `resolveVaultPath()`. It can be passed to the `Indexing` constructor.

### R7: Slash Command Convention

Existing slash commands in `.opencode/command/` are markdown files that instruct the agent. They contain:
- A description of what the command does
- Instructions for the agent to call specific MCP tools
- Expected output format

The `dewey-index.md` and `dewey-reindex.md` files follow this pattern — they instruct the agent to call the `index` or `reindex` MCP tool and display the result.

## Phase 1: Design

### D1: Indexing Struct

```go
// tools/indexing.go

type Indexing struct {
    mu        sync.Mutex
    embedder  embed.Embedder
    store     *store.Store
    vaultPath string
}

func NewIndexing(e embed.Embedder, s *store.Store, vaultPath string) *Indexing {
    return &Indexing{embedder: e, store: s, vaultPath: vaultPath}
}
```

The `sync.Mutex` provides mutual exclusion. The `vaultPath` enables finding `sources.yaml` and the cache directory.

### D2: Index Handler

```go
func (idx *Indexing) Index(ctx context.Context, req *mcp.CallToolRequest, input IndexInput) (*mcp.CallToolResult, any, error)
```

Input: `IndexInput{SourceID string}` (optional — empty means all sources).

Flow:
1. Validate store is non-nil (return error result if nil)
2. Acquire mutex with `TryLock()` — return error if already locked
3. Defer `Unlock()`
4. Load source configs from `sources.yaml`
5. Build last-fetched times from store
6. Create source manager, call `FetchAll(input.SourceID, false, lastFetchedTimes)`
7. Call `indexDocuments()` (the existing function from `cli.go`)
8. Return structured JSON summary

### D3: Reindex Handler

```go
func (idx *Indexing) Reindex(ctx context.Context, req *mcp.CallToolRequest, input struct{}) (*mcp.CallToolResult, any, error)
```

Flow:
1. Validate store is non-nil
2. Acquire mutex with `TryLock()`
3. Defer `Unlock()`
4. Load source configs from `sources.yaml`
5. For each config where `cfg.ID != "disk-local"` and `cfg.ID != "learning"`: call `store.DeletePagesBySource(cfg.ID)`
6. Create source manager, call `FetchAll("", true, emptyLastFetched)` — force mode, all sources
7. Call `indexDocuments()` with the fetched documents
8. Return structured JSON rebuild summary

### D4: Server Wiring

In `server.go`, add `registerIndexingTools()` following the pattern of `registerLearningTools()`. The indexing tools are write operations (they modify the store), so they are only registered when `!readOnly`. They also require the vault path, which is available from the `serverConfig` — we add a `vaultPath` field to `serverConfig`.

```go
// In newServer():
if !readOnly {
    indexing := tools.NewIndexing(cfg.embedder, cfg.store, cfg.vaultPath)
    toolCount += registerIndexingTools(srv, indexing)
}
```

New server option:
```go
func WithVaultPath(p string) serverOption {
    return func(c *serverConfig) { c.vaultPath = p }
}
```

### D5: Input Types

Add to `types/tools.go`:
```go
type IndexInput struct {
    SourceID string `json:"source_id,omitempty"`
}

type ReindexInput struct{}
```

### D6: indexDocuments Extraction

The `indexDocuments()` function currently lives in `cli.go` (package `main`). The MCP tools in `tools/` cannot call it directly because it's in a different package. Two options:

**Option A**: Move `indexDocuments()` to a shared package (e.g., `vault/` or a new `indexer/` package).
**Option B**: Keep `indexDocuments()` in `cli.go` and duplicate the logic in `tools/indexing.go`.
**Option C**: Keep `indexDocuments()` in `cli.go` and have the `Indexing` struct accept a function parameter for the indexing pipeline.

**Decision**: Option A is cleanest but creates a new package or moves code to `vault/` which already has a different responsibility. Option C is the most testable — the `Indexing` struct accepts an `IndexFunc` that encapsulates the pipeline. However, looking more carefully, the `indexDocuments` function has many dependencies (`source.SourceConfig`, `source.Document`, `store.Store`, `embed.Embedder`, `vault.ParseDocument`, `vault.PersistBlocks`, `vault.PersistLinks`, `vault.GenerateEmbeddings`). All of these are already importable from `tools/`.

**Revised Decision**: The `Indexing` struct in `tools/indexing.go` will implement its own indexing pipeline that calls the same shared functions (`vault.ParseDocument`, `vault.PersistBlocks`, `vault.PersistLinks`, `vault.GenerateEmbeddings`). This is not duplication — it's the same underlying functions, just orchestrated from a different call site. The orchestration logic (loop over documents, upsert pages, update source records) is ~50 lines and is specific to each call site's needs (CLI has different error handling and logging than MCP tools).

This approach:
- Avoids cross-package coupling with `package main`
- Allows the MCP tool to return structured results (not just log output)
- Enables the MCP tool to report per-source progress in the response
- Is fully testable with in-memory store and mock embedder

### D7: Response Structure

Both tools return a JSON response matching FR-004:

```json
{
  "status": "completed",
  "sources_processed": 3,
  "pages": {
    "new": 12,
    "changed": 3,
    "deleted": 0
  },
  "embeddings_generated": 15,
  "embeddings_skipped": 0,
  "elapsed_ms": 2340,
  "sources": [
    {
      "id": "github-unbound-force",
      "type": "github",
      "documents": 8,
      "error": ""
    }
  ]
}
```

### D8: Tool Count Impact

Current tool counts (from `server_test.go`):
- Non-DataScript read-write: 30–35 (currently 34: nav=5, search=3, analyze=5, write=10, decision=5, journal=2, semantic=3, health=1)
- Adding 2 indexing tools → 36
- Plus `store_learning` (1) → already counted

The test ranges in `TestNewServer_ToolCount` need updating:
- "non-DataScript read-write": wantMin=30→32, wantMax=35→37
- "DataScript read-write": wantMin=38→40, wantMax=42→44
- "vault read-write": wantMin=31→33, wantMax=36→38

Read-only modes are unaffected (indexing tools are write-only).

### D9: Slash Command Design

`/dewey-index` instructs the agent to:
1. Call the `index` MCP tool (optionally with a `source_id` parameter)
2. Display the returned summary

`/dewey-reindex` instructs the agent to:
1. Warn the user that this deletes and rebuilds all external source content
2. Call the `reindex` MCP tool
3. Display the rebuild summary

## Coverage Strategy

| Component | Test Approach | Coverage Target |
|-----------|--------------|-----------------|
| `Indexing.Index` happy path | In-memory store, mock source config, verify pages created | Contract |
| `Indexing.Index` with source_id filter | Verify only specified source is indexed | Contract |
| `Indexing.Index` nil store | Verify error result | Error path |
| `Indexing.Index` concurrent call | Verify mutex rejection | Concurrency |
| `Indexing.Reindex` happy path | Pre-populate store, verify external pages deleted and re-created | Contract |
| `Indexing.Reindex` preserves disk-local | Verify disk-local pages survive reindex | Safety |
| `Indexing.Reindex` preserves learning | Verify learning pages survive reindex | Safety |
| `Indexing.Reindex` concurrent call | Verify mutex rejection | Concurrency |
| `Indexing` embedder unavailable | Verify graceful degradation | Degradation |
| Server registration | Verify tools appear in tool list | Integration |
| Server read-only mode | Verify indexing tools excluded | Integration |
| Tool count | Verify updated ranges | Regression |

## Complexity Tracking

No constitution violations. All design choices align with existing patterns:
- `Indexing` struct follows `Learning` struct pattern (DIP)
- `sync.Mutex` is the simplest correct mutual exclusion primitive
- Slash commands are thin `.md` wrappers per existing convention
- No new packages, no new dependencies, no CGO
