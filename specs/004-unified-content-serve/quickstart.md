# Quickstart: Unified Content Serve

**Date**: 2026-03-28

## Prerequisites

- Go 1.25+
- Dewey repository cloned and buildable (`go build ./...`)
- A configured `.dewey/sources.yaml` with at least one external source (GitHub or web)
- Optional: Ollama running locally for semantic search (`ollama serve`)

## Development Setup

```bash
# Build dewey
go build ./...

# Run tests (always with race detection)
go test -race -count=1 ./...

# Run tests with coverage for Gaze
go test -race -count=1 -coverprofile=coverage.out ./...

# Static analysis
go vet ./...
```

## Verifying the Feature

### 1. Index external content

```bash
# Initialize dewey in a vault directory
dewey init --vault .

# Configure a GitHub source in .dewey/sources.yaml
# (see README.md for source configuration format)

# Run indexing — now persists blocks, links, and embeddings
dewey index

# Expected output (structured log):
# INFO dewey: indexing source source=github-myorg
# INFO dewey: parsing documents documents=42
# INFO dewey: persisting blocks blocks=187 links=23
# INFO dewey: generating embeddings blocks=187 (if Ollama available)
# INFO dewey: index complete documents=42 errors=0
```

### 2. Verify content in store

```bash
# Check status — now shows per-source counts
dewey status

# Expected: Pages by source breakdown showing disk-local AND external sources
```

### 3. Serve and query

```bash
# Start MCP server (Obsidian backend)
dewey serve --backend obsidian --vault .

# Expected log on startup:
# INFO dewey: loaded external pages from store count=42
# INFO dewey: vault ready pages=1165 (1123 local + 42 external)
```

### 4. Test write protection

When an MCP client tries to edit an external page:
```json
// Request: dewey_update_block on a GitHub issue block
// Response:
{
  "error": "page \"github-myorg/issues/42\" is read-only (source: github-myorg)"
}
```

## Key Files to Understand

| File | Purpose |
|------|---------|
| `vault/parse_export.go` | NEW: Exported `ParseDocument()` for use by `dewey index` |
| `vault/vault_store.go` | `LoadExternalPages()`, `reconstructBlockTree()` |
| `vault/vault.go` | `cachedPage` struct changes (sourceID, readOnly), write guards |
| `store/store.go` | New methods: `ListPagesExcludingSource`, `DeletePagesBySource`, `ListPagesBySource` |
| `cli.go` | `indexDocuments()` upgraded to persist blocks/links/embeddings |
| `main.go` | Startup sequence: `LoadExternalPages()` call after vault indexing |

## Testing Strategy

```bash
# Run all tests
go test -race -count=1 ./...

# Run specific package tests
go test -race -count=1 ./vault/...     # Vault loading, write guards, tree reconstruction
go test -race -count=1 ./store/...     # New store methods, source-level operations
go test -race -count=1 ./...           # Integration: CLI index + serve round-trip

# Run with coverage for Gaze quality gates
go test -race -count=1 -coverprofile=coverage.out ./...
gaze report ./... --coverprofile=coverage.out --max-crapload=48 --max-gaze-crapload=18 --min-contract-coverage=70
```
