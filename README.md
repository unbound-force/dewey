# Dewey

Knowledge graph MCP server that gives AI full access to your knowledge graph. Supports **Logseq** and **Obsidian** — both with full read-write support. Navigate pages, search blocks, analyze link structure, track decisions, manage flashcards, and write content — all through the [Model Context Protocol](https://modelcontextprotocol.io).

Hard fork of [graphthulhu](https://github.com/skridlevsky/graphthulhu) by Max Skridlevsky, extended with persistence, semantic search, and pluggable content sources for the [Unbound Force](https://github.com/unbound-force) AI agent swarm ecosystem.

Built in Go with the [official MCP Go SDK](https://github.com/modelcontextprotocol/go-sdk).

## Why

Your knowledge graph stores interconnected pages, blocks, and links. But AI assistants can't see any of it — they're blind to your second brain.

Dewey fixes that. It exposes your entire knowledge graph through MCP, so any MCP client can:

- Read any page with its full block tree, parsed links, tags, and properties
- Search across all blocks with contextual results (parent chain + siblings)
- Traverse the link graph to discover how concepts connect
- Find knowledge gaps — orphan pages, dead ends, weakly-linked areas
- Discover topic clusters through connected component analysis
- Create pages, write blocks, build hierarchies, link pages bidirectionally
- Query with raw DataScript/Datalog for anything the built-in tools don't cover (Logseq)
- Review flashcards with spaced repetition statistics (Logseq)
- Explore whiteboards and their spatial connections (Logseq)

It turns "tell me about X" into an AI that actually understands your knowledge graph's structure.

## Tools

40 tools across 10 categories. Most work with both backends; some are Logseq-only (DataScript queries, flashcards, whiteboards).

### Navigate

| Tool | Backend | Description |
|------|---------|-------------|
| `get_page` | Both | Full recursive block tree with parsed links, tags, properties |
| `get_block` | Both | Block by UUID with ancestor chain, children, siblings |
| `list_pages` | Both | Filter by namespace, property, or tag; sort by name/modified/created |
| `get_links` | Both | Forward and backward links with the blocks that contain them |
| `get_references` | Logseq | All blocks referencing a specific block via `((uuid))` |
| `traverse` | Both | BFS path-finding between two pages through the link graph |

### Search

| Tool | Backend | Description |
|------|---------|-------------|
| `search` | Both | Full-text search with parent chain + sibling context |
| `query_properties` | Both | Find by property values with operators (eq, contains, gt, lt) |
| `query_datalog` | Logseq | Raw DataScript/Datalog queries against the Logseq database |
| `find_by_tag` | Both | Tag search with child tag hierarchy support |

### Analyze

| Tool | Backend | Description |
|------|---------|-------------|
| `graph_overview` | Both | Global stats: pages, blocks, links, most connected, namespaces |
| `find_connections` | Both | Direct links, shortest paths, shared connections between pages |
| `knowledge_gaps` | Both | Orphan pages, dead ends, weakly-linked areas |
| `list_orphans` | Both | List orphan page names with block counts and property status |
| `topic_clusters` | Both | Connected components with hub identification |

### Write

| Tool | Backend | Description |
|------|---------|-------------|
| `create_page` | Both | New page with properties and initial blocks |
| `append_blocks` | Both | Append plain-text blocks (simpler than upsert_blocks) |
| `upsert_blocks` | Both | Batch create with nested children for deep hierarchies |
| `update_block` | Both | Replace block content by UUID |
| `delete_block` | Both | Remove block and all children |
| `move_block` | Both | Reposition before, after, or as child of another block (cross-page supported) |
| `link_pages` | Both | Bidirectional link with optional relationship context |
| `delete_page` | Both | Remove a page and all its blocks |
| `rename_page` | Both | Rename page and update all `[[links]]` across the graph |
| `bulk_update_properties` | Both | Set a property on multiple pages in one call |

### Decision

| Tool | Backend | Description |
|------|---------|-------------|
| `decision_check` | Both | Surface open, overdue, and resolved decisions with deadline status |
| `decision_create` | Both | Create a DECIDE block with `#decision` tag, deadline, options, context |
| `decision_resolve` | Both | Mark a decision as DONE with resolution date and outcome |
| `decision_defer` | Both | Push deadline with reason, tracks deferral count, warns after 3+ |
| `analysis_health` | Both | Audit analysis/strategy pages for graph connectivity (3+ links or has decision) |

### Journal

| Tool | Backend | Description |
|------|---------|-------------|
| `journal_range` | Both | Entries across a date range with full block trees |
| `journal_search` | Both | Search within journals, optionally filtered by date |

### Flashcard

| Tool | Backend | Description |
|------|---------|-------------|
| `flashcard_overview` | Logseq | SRS stats: total, due, new vs reviewed, average repeats |
| `flashcard_due` | Logseq | Cards due for review with ease factor and interval |
| `flashcard_create` | Logseq | Create front/back card with `#card` tag |

### Whiteboard

| Tool | Backend | Description |
|------|---------|-------------|
| `list_whiteboards` | Logseq | All whiteboards in the graph |
| `get_whiteboard` | Logseq | Embedded pages, block references, visual connections |

### Semantic Search

| Tool | Backend | Description |
|------|---------|-------------|
| `dewey_semantic_search` | Both | Find documents semantically similar to a natural language query |
| `dewey_similar` | Both | Find the most similar documents to a given page or block |
| `dewey_semantic_search_filtered` | Both | Semantic search with metadata filters (source, properties) |

### Health

| Tool | Backend | Description |
|------|---------|-------------|
| `health` | Both | Check server status: version, backend, read-only mode, page count, embedding status, sources |

## Install

### Homebrew (macOS)

```bash
brew install --cask unbound-force/tap/dewey
```

The cask includes a signed and notarized binary. Ollama is installed automatically as a dependency.

### go install (all platforms)

```bash
go install github.com/unbound-force/dewey@latest
```

Requires Go 1.25+. Works on macOS, Linux, and any platform with a Go toolchain.

### Build from source

```bash
git clone https://github.com/unbound-force/dewey.git
cd dewey
go build -o dewey .
```

### Setup: Logseq

1. In Logseq, go to **Settings → Features** and enable **HTTP APIs server**
2. Click the **API** icon that appears in the top toolbar
3. Click **Start Server**
4. Click **Create Token** and copy the generated token — you'll need it for configuration

The API runs on `http://127.0.0.1:12315` by default.

### Setup: Obsidian

No plugins or server required. Dewey reads your vault's `.md` files directly.

You need to provide the path to your vault:

```bash
dewey serve --backend obsidian --vault /path/to/your/vault
```

Or via environment variables:

```bash
export DEWEY_BACKEND=obsidian
export OBSIDIAN_VAULT_PATH=/path/to/your/vault
dewey
```

The Obsidian backend supports full read-write operations. It parses YAML frontmatter into properties, builds a block tree from headings, and indexes `[[wikilinks]]` for backlink resolution. Writes use atomic temp-file renames, and the in-memory index is rebuilt after every mutation. File watching (fsnotify) keeps the index in sync with external edits. Daily notes are detected from a configurable subfolder (default: `daily notes`).

## Configuration

### Logseq — OpenCode

Add to your `opencode.json`:

```json
{
  "mcp": {
    "dewey": {
      "type": "local",
      "command": ["dewey"],
      "env": {
        "LOGSEQ_API_URL": "http://127.0.0.1:12315",
        "LOGSEQ_API_TOKEN": "your-token-here"
      }
    }
  }
}
```

### Obsidian — OpenCode

```json
{
  "mcp": {
    "dewey": {
      "type": "local",
      "command": ["dewey", "--backend", "obsidian", "--vault", "/path/to/your/vault"]
    }
  }
}
```

### Read-only mode

To disable all write operations (Obsidian backend, the default):

```json
{
  "mcp": {
    "dewey": {
      "type": "local",
      "command": ["dewey", "--read-only", "--backend", "obsidian", "--vault", "/path/to/your/vault"]
    }
  }
}
```

For the Logseq backend (requires Logseq running with API enabled):

```json
{
  "mcp": {
    "dewey": {
      "type": "local",
      "command": ["dewey", "--read-only", "--backend", "logseq"],
      "env": {
        "LOGSEQ_API_URL": "http://127.0.0.1:12315",
        "LOGSEQ_API_TOKEN": "your-token-here"
      }
    }
  }
}
```

### Version control warning

On startup with the Logseq backend, Dewey checks if your graph directory is git-controlled. If not, it prints a warning to stderr suggesting you initialize version control. Write operations cannot be undone without it.

### Persistence

Dewey stores its index in `.uf/dewey/graph.db` (SQLite). The database holds pages, blocks, links, vector embeddings, and source metadata. After the first full index, subsequent sessions load from the persistent index and only reprocess changed files — startup is near-instant.

Run `dewey init` to create the `.uf/dewey/` directory with default configuration:

```bash
dewey init
```

This creates:
- `.uf/dewey/config.yaml` — embedding model and endpoint settings
- `.uf/dewey/sources.yaml` — content source configuration (empty by default)
- `.uf/dewey/graph.db` — created automatically on first `dewey serve` or `dewey index`
- `.uf/dewey/dewey.log` — created automatically by `dewey serve` for MCP server diagnostics (truncated at 10 MB on startup)

Add `.uf/dewey/` to your `.gitignore`. The index is machine-local and rebuilt from source files.

### Environment variables

| Variable | Default | Description |
|----------|---------|-------------|
| `LOGSEQ_API_URL` | `http://127.0.0.1:12315` | Logseq HTTP API endpoint |
| `LOGSEQ_API_TOKEN` | (required for Logseq) | Bearer token from Logseq settings |
| `DEWEY_BACKEND` | `obsidian` | Backend type: `obsidian` (default) or `logseq` |
| `OBSIDIAN_VAULT_PATH` | — | Path to Obsidian vault root |
| `DEWEY_EMBEDDING_MODEL` | `granite-embedding:30m` | Ollama embedding model name |
| `DEWEY_EMBEDDING_ENDPOINT` | `http://localhost:11434` | Ollama API endpoint |
| `GITHUB_TOKEN` / `GH_TOKEN` | — | GitHub API token for content sources |

### Global flags

| Flag | Short | Description |
|------|-------|-------------|
| `--verbose` | `-v` | Enable debug logging (shows UUID seeds, block insertions, lock detection) |
| `--log-file PATH` | | Write logs to file in addition to stderr |

When running as an MCP server (`dewey serve`), Dewey automatically logs to `.uf/dewey/dewey.log` for diagnostics. The log file is truncated when it exceeds 10 MB.

## CLI Commands

### dewey init

Initialize a Dewey configuration in the current directory. Creates `.uf/dewey/` with default `config.yaml` and `sources.yaml`.

```bash
dewey init [--vault PATH]
```

### dewey index

Build or update the knowledge graph and embedding indexes from all configured sources. Uses content hashing for incremental updates — only re-indexes changed files.

```bash
dewey index [--vault PATH] [--source NAME] [--force] [--no-embeddings]
```

- `--force` — Re-fetch all sources, even if within their refresh interval
- `--no-embeddings` — Skip embedding generation (keyword search still works)

### dewey reindex

Delete the existing index and rebuild from scratch. Use when upgrading Dewey, recovering from corruption, or after fixing UUID collision issues.

```bash
dewey reindex [--vault PATH] [--no-embeddings]
```

This removes `graph.db`, WAL/SHM files, and the lock file before rebuilding. Requires stopping `dewey serve` first (the lock file prevents concurrent access).

### dewey status

Report index health: page count, block count, embedding coverage, source status.

```bash
dewey status [--vault PATH] [--json]
```

### dewey doctor

Run diagnostic checks for Dewey dependencies and report pass/fail with fix instructions. Checks: workspace initialization, database health (per-source page counts), Ollama availability, embedding model status, MCP server process, and opencode.json configuration.

```bash
dewey doctor [--vault PATH]
```

### dewey search

Full-text search across the knowledge graph (local files and external sources from graph.db).

```bash
dewey search [--vault PATH] [--limit N] QUERY
```

### dewey source add

Add a content source (GitHub or web) to the configuration.

```bash
dewey source add github --org ORG --repos REPO1,REPO2 [--refresh INTERVAL]
dewey source add web --url URL [--name NAME] [--depth N] [--refresh INTERVAL]
```

## Content Sources

Dewey indexes content from three pluggable source types. Configure them in `.uf/dewey/sources.yaml`:

```yaml
sources:
  - name: local-vault
    type: disk
    config:
      path: .

  - name: org-repos
    type: github
    config:
      org: your-org
      repos:
        - repo-one
        - repo-two
      content_types:
        - issues
        - pulls
        - readmes
    refresh: daily

  - name: go-docs
    type: web
    config:
      urls:
        - https://pkg.go.dev/github.com/spf13/cobra
      depth: 2
    refresh: weekly
```

You can also add sources via the CLI instead of editing YAML directly:

```bash
dewey source add github --org your-org --repos repo-one,repo-two
dewey source add web --url https://pkg.go.dev/github.com/spf13/cobra --depth 2
```

After adding sources, build the index:

```bash
dewey index             # incremental — only fetches sources past their refresh interval
dewey index --force     # full rebuild — re-fetches everything
```

## Semantic Search Setup

Semantic search requires [Ollama](https://ollama.ai) running locally. All 37 keyword-based tools work without it — only the 3 semantic search tools (`dewey_semantic_search`, `dewey_similar`, `dewey_semantic_search_filtered`) require Ollama.

### Install Ollama and pull the embedding model

```bash
brew install --cask ollama-app  # macOS — or download from https://ollama.ai
ollama serve              # start the Ollama server (runs in background)
ollama pull granite-embedding:30m   # IBM Granite, 63 MB, Apache 2.0
```

### Verify

```bash
dewey status
```

The output shows embedding coverage. If Ollama is running and the model is pulled, you'll see embedding stats. If Ollama is unavailable, Dewey logs a warning at startup and disables semantic search — all other tools continue to work normally.

### Configuration

The embedding model and endpoint are configurable via environment variables or `.uf/dewey/config.yaml`:

| Variable | Default | Description |
|----------|---------|-------------|
| `DEWEY_EMBEDDING_MODEL` | `granite-embedding:30m` | Ollama model name |
| `DEWEY_EMBEDDING_ENDPOINT` | `http://localhost:11434` | Ollama API endpoint |

## Architecture

```
main.go              Entry point — backend routing, MCP server startup
cli.go               CLI subcommands: journal, add, search, init, index, status, source
server.go            MCP server setup — conditional tool registration
backend/backend.go   Backend interface + optional capability interfaces
client/logseq.go     Logseq HTTP API client with retry/backoff
vault/
  vault.go           Obsidian vault client — reads .md files into Backend interface
  markdown.go        Markdown → block tree parser (heading-based sectioning)
  frontmatter.go     YAML frontmatter parser
  index.go           Backlink index builder from [[wikilinks]]
tools/
  navigate.go        Page, block, links, references, BFS traversal
  search.go          Full-text, property, DataScript/frontmatter, tag search
  analyze.go         Graph overview, connections, gaps, clusters
  write.go           Create, update, delete, move, link operations
  decision.go        Decision protocol: check, create, resolve, defer, analysis health
  journal.go         Date range and search within journals
  flashcard.go       SRS overview, due cards, card creation
  whiteboard.go      List and inspect whiteboards
  helpers.go         Result formatting utilities
graph/
  builder.go         In-memory graph construction from any backend
  algorithms.go      Overview, connections, gaps, clusters, BFS
parser/content.go    Regex extraction of [[links]], ((refs)), #tags, properties
types/
  logseq.go          Shared types with custom JSON unmarshaling
  tools.go           Input types for all 40 tools
store/
  store.go           SQLite persistence (pages, blocks, links, sources)
  embeddings.go      Vector embedding storage and cosine similarity search
  migrate.go         Schema migration management
embed/
  embed.go           Embedder interface + Ollama implementation
  chunker.go         Block-to-chunk preparation with heading context
source/
  source.go          Source interface definition
  config.go          Source configuration parsing (YAML)
  disk.go            Local disk source (file scanning)
  github.go          GitHub API source (issues, PRs, READMEs)
  web.go             Web crawl source (HTML-to-text, robots.txt)
  manager.go         Source orchestration (refresh, failures)
```

## Attribution

Dewey is a hard fork of [graphthulhu](https://github.com/skridlevsky/graphthulhu), originally created by [Max Skridlevsky](https://github.com/skridlevsky). All graphthulhu functionality is preserved; Dewey extends it with additional capabilities for the Unbound Force ecosystem.

## Development

```bash
go build -o dewey .          # Build
go test ./...                # Test
go vet ./...                 # Vet
```

## License

[MIT](LICENSE)
