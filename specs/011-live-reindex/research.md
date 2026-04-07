# Research: Live Reindex

**Branch**: `011-live-reindex` | **Date**: 2026-04-07

## R1: Existing Indexing Pipeline

The `indexDocuments()` function in `cli.go` (line 947) is the authoritative indexing pipeline used by both `dewey index` and `dewey reindex` CLI commands. It accepts:
- `*store.Store` — SQLite persistence layer
- `map[string][]source.Document` — fetched documents grouped by source ID
- `[]source.SourceConfig` — source configurations for metadata
- `embed.Embedder` — optional embedding generator

For each document, it:
1. Namespaces the page name as `sourceID/docID` (lowercase)
2. Parses content via `vault.ParseDocument(pageName, doc.Content)`
3. Upserts the page record in the store
4. Persists blocks via `vault.PersistBlocks()`
5. Persists links via `vault.PersistLinks()`
6. Generates embeddings via `vault.GenerateEmbeddings()` (if embedder available)
7. Updates source records (`InsertSource`/`UpdateLastFetched`/`UpdateSourceStatus`)

**Key finding**: This function lives in `package main` and cannot be imported by `tools/`. The MCP tool must implement its own orchestration calling the same shared functions from `vault/` and `store/`.

## R2: Source Manager Lifecycle

`source.NewManager(configs, basePath, cacheDir)` instantiates source implementations (disk, github, web, code) from config. `mgr.FetchAll(sourceName, force, lastFetchedTimes)` returns `(*FetchResult, map[string][]Document)`.

The source config lives at `.uf/dewey/sources.yaml` and is loaded via `source.LoadSourcesConfig(path)`. The MCP tools need the vault path to construct the full path to this file.

## R3: In-Process Database Access

When `dewey serve` runs, it opens `*store.Store` during `initObsidianBackend()` and holds it for the process lifetime. The store is passed to MCP tools via `serverConfig`. SQLite WAL mode allows concurrent reads during writes, so search/navigate tools remain functional during indexing (FR-006).

No lock conflict exists because the MCP tools run in the same process as the server — they share the same `*store.Store` instance.

## R4: Mutual Exclusion

`sync.Mutex` with `TryLock()` (Go 1.18+) provides non-blocking mutual exclusion. If `TryLock()` returns false, the handler immediately returns an error result. This is simpler than `sync/atomic` because the mutex also provides memory ordering for the indexing state.

## R5: Reindex Safety — Protected Source IDs

The `reindex` MCP tool must NOT delete pages from these source IDs:
- `"disk-local"` — local vault content loaded by `IncrementalIndex` at serve startup
- `"learning"` — agent learnings stored via `store_learning` MCP tool

All other source IDs (e.g., `"github-unbound-force"`, `"web-go-stdlib"`) are external and safe to delete-and-rebuild.

The CLI `reindex` command deletes the entire `graph.db` file and rebuilds from scratch. The MCP `reindex` tool cannot do this (the server is using the database), so it uses per-source `store.DeletePagesBySource()` instead. This is a safer, more targeted approach.

## R6: Vault Path Propagation

The vault path flows through the system as:
1. `--vault` flag or `OBSIDIAN_VAULT_PATH` env var → `resolveVaultPath()`
2. `initObsidianBackend()` uses it for store, vault client, and source config
3. `newServer()` receives it indirectly via `serverConfig`

For the indexing tools, we add `vaultPath` to `serverConfig` and pass it via a new `WithVaultPath()` option. The `Indexing` struct stores it and uses it to find `sources.yaml` and the cache directory.

## R7: Slash Command Convention

Existing slash commands in `.opencode/command/` are markdown files containing:
- A title and description
- Instructions for the agent (what MCP tool to call, what parameters to pass)
- Expected behavior and output format

Examples: `gaze.md`, `review-council.md`, `cobalt-crush.md`. The dewey indexing commands follow this same pattern.

## R8: `reportSourceErrors` and `purgeOrphanedSources`

Two helper functions in `cli.go` are relevant:
- `reportSourceErrors(s, result)` — updates source status for failed fetches
- `purgeOrphanedSources(s, configs)` — removes pages for sources no longer in config

The MCP tools should call equivalent logic. Since these are simple store operations, they can be reimplemented in `tools/indexing.go` without significant duplication.
