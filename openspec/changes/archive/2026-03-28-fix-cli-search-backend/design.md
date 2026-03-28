## Context

The `dewey search` CLI command (`cli.go:127-179`) creates a Logseq HTTP client via `client.New("", "")` and calls `GetAllPages()` + `GetPageBlocksTree()` to iterate all pages and match content. This path is completely disconnected from the vault backend used by `dewey serve`. Since no user runs Logseq, search always returns zero results.

The vault backend already has `FullTextSearch(query string, limit int)` which returns matching blocks with page context. The serve command successfully uses this for the `dewey_full_text_search` MCP tool.

## Goals / Non-Goals

### Goals
- `dewey search` returns results from local vault files
- Search uses the same vault backend as `dewey serve`
- If a persistent store exists (`.dewey/graph.db`), search also includes external-source pages loaded from the store

### Non-Goals
- Changing the MCP search tools (they already work correctly via serve)
- Adding new search features (semantic search, filtered search)
- Modifying the vault's `FullTextSearch()` implementation

## Decisions

**D1: Replace Logseq client with vault backend in `newSearchCmd()`**

The search command will:
1. Resolve the vault path using the same logic as `initObsidianBackend()` (`--vault` flag or `DEWEY_VAULT_PATH` env var)
2. Create a `vault.Client`, call `Load()`, and optionally load external pages from graph.db
3. Call `vault.Client.FullTextSearch(query, limit)` instead of iterating pages via the Logseq client
4. Print results using the existing `printSearchResults()` function (adapted for the vault's result format)

**D2: Reuse `resolveBackendType()` for backend detection**

The search command will check `resolveBackendType()`. If the backend is `logseq`, it falls back to the existing Logseq client path for backward compatibility. If `obsidian` (the default), it uses the vault backend. This preserves the option for Logseq users while fixing the common case.

**D3: No startup of the full MCP server**

The search command creates a lightweight vault client — it does NOT start the MCP server, register tools, or run the file watcher. It's a one-shot query: create vault, load, search, print, exit.

## Risks / Trade-offs

**Low risk**: The vault's `FullTextSearch()` is already battle-tested via the MCP tool path. The change replaces a non-functional code path (Logseq client that always fails) with a functional one (vault backend).

**Startup latency**: Creating and loading the vault adds a small delay (~100-500ms depending on vault size) vs the instant (but useless) Logseq client path. Acceptable for a CLI command.
