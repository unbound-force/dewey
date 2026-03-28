## Why

The `dewey search` CLI command always returns zero results because it is hardwired to the Logseq HTTP client (`client.New("", "")` at `cli.go:142`), which tries to connect to a Logseq API on `localhost:12315`. It completely ignores the `--backend` flag and never uses the Obsidian vault backend. Since no Dewey user runs Logseq, every `dewey search` invocation silently returns nothing.

The fix is straightforward: rewrite `newSearchCmd()` to instantiate the vault backend (same as `dewey serve`) and use its `FullTextSearch()` method instead of going through the Logseq HTTP client.

## What Changes

- Rewrite `newSearchCmd()` in `cli.go` to use the vault backend with `FullTextSearch()` instead of the Logseq HTTP client
- The search command will respect the `--vault` and `--backend` flags from the root command
- Remove the Logseq client dependency from the search path

## Capabilities

### New Capabilities
- None

### Modified Capabilities
- `cli-search`: The `dewey search` command now uses the vault backend (same data path as `dewey serve`) instead of the Logseq HTTP client. Returns results from local `.md` files and graph.db external content.

### Removed Capabilities
- None

## Impact

- **`cli.go`**: Rewrite `newSearchCmd()` to create an Obsidian vault, call `Load()`, optionally load external pages from graph.db, and use `FullTextSearch()` for queries
- **`cli_test.go`**: Update search command tests to verify the new vault-based search path
- **User impact**: `dewey search` will actually return results for the first time

## Constitution Alignment

Assessed against the Unbound Force org constitution.

### I. Autonomous Collaboration

**Assessment**: N/A

This change modifies a CLI command's internal implementation. No MCP tools, tool contracts, or inter-agent communication are affected.

### II. Composability First

**Assessment**: PASS

Dewey remains independently installable. The search command uses the same vault backend as serve — no new dependencies. The Logseq client remains available for users who explicitly choose `--backend logseq`.

### III. Observable Quality

**Assessment**: PASS

Search results continue to include page names and block content. The fix ensures results are actually returned (previously zero results was the "output"), improving observable quality.

### IV. Testability

**Assessment**: PASS

The vault backend is testable with `t.TempDir()` and in-memory SQLite. The rewritten search command can be tested by creating a temp vault with `.md` files and verifying search returns matches — no external services required.
