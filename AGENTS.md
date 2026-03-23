# dewey Development Guidelines

Auto-generated from all feature plans. Last updated: 2026-03-22

## Active Technologies
- SQLite via `modernc.org/sqlite` -- single database `.dewey/graph.db` containing the knowledge graph index (pages, blocks, links) and vector embeddings (001-core-implementation)

- Go 1.25 (per `go.mod`) + `modernc.org/sqlite` v1.47.0 (pure-Go SQLite), `github.com/k3a/html2text` v1.4.0 (HTML-to-text for web crawl), `github.com/modelcontextprotocol/go-sdk` v1.2.0 (existing MCP SDK), `github.com/spf13/cobra` (CLI framework), `github.com/charmbracelet/log` (structured logging) (001-core-implementation)

## Project Structure

```text
main.go              # Entry point, CLI routing
cli.go               # CLI subcommands (journal, add, search, init, index, status)
server.go            # MCP server setup, tool registration
backend/             # Backend interface + capability interfaces
client/              # Logseq HTTP API client
vault/               # Obsidian vault backend (file parsing, indexing, watcher)
tools/               # MCP tool implementations (navigate, search, analyze, write, etc.)
types/               # Shared types (PageEntity, BlockEntity, tool inputs)
parser/              # Content parser (wikilinks, tags, properties)
graph/               # In-memory graph construction + algorithms
store/               # SQLite persistence layer (pages, blocks, links, embeddings)
embed/               # Embedding generation (Ollama client, chunker)
source/              # Pluggable content sources (disk, GitHub, web crawl)
```

## Commands

```bash
go build ./...       # Build all packages
go test ./...        # Run all tests
go vet ./...         # Run static analysis
```

## Code Style

Go 1.25 (per `go.mod`): Follow standard conventions

## Recent Changes
- 001-core-implementation: Added Go 1.25 (per `go.mod`) + `modernc.org/sqlite` v1.47.0 (pure-Go SQLite), `github.com/k3a/html2text` v1.4.0 (HTML-to-text for web crawl), `github.com/modelcontextprotocol/go-sdk` v1.2.0 (existing MCP SDK), `github.com/spf13/cobra` (CLI framework), `github.com/charmbracelet/log` (structured logging)

- 001-core-implementation: Added Go 1.25 (per `go.mod`) + `modernc.org/sqlite` v1.47.0 (pure-Go SQLite), `github.com/k3a/html2text` v1.4.0 (HTML-to-text for web crawl), `github.com/modelcontextprotocol/go-sdk` v1.2.0 (existing MCP SDK)

<!-- MANUAL ADDITIONS START -->
<!-- MANUAL ADDITIONS END -->
<!-- scaffolded by unbound vdev -->
<!-- scaffolded by unbound vdev -->
