# Quickstart: Code Source Indexing

## Configure a Code Source

Add to `.dewey/sources.yaml`:

```yaml
sources:
  - id: code-replicator
    type: code
    name: replicator
    config:
      path: "../replicator"
      languages: [go]
      # include: ["cmd/", "internal/"]     # optional: only index these paths
      # exclude: ["vendor/", "testdata/"]  # optional: skip these paths
      # ignore: ["generated_*.go"]         # optional: extra gitignore patterns
      # recursive: true                    # default: true
```

## Index Code Sources

```bash
# Index all sources (including code sources)
dewey index

# Index only a specific code source
dewey index --source code-replicator

# Index without embeddings (faster, no semantic search)
dewey index --no-embeddings
```

## Search Code

After indexing, use semantic search to find CLI commands, exported types, and functions:

```bash
# Via MCP tools (from an AI agent)
# semantic_search({ query: "what CLI commands does replicator have" })
# semantic_search({ query: "exported types in engine package" })

# Via CLI
dewey search "replicator CLI commands"
```

## Generate a Manifest

Run in any Go project to generate a summary of its public interface:

```bash
dewey manifest
# Creates .dewey/manifest.md with:
#   - CLI Commands (Cobra)
#   - MCP Tools
#   - Exported Packages
```

The manifest is a regular markdown file — it gets indexed automatically by any disk source pointing to this repo.

## Supported Languages

| Language | Status | File Extensions | Test Pattern |
|----------|--------|-----------------|--------------|
| Go       | ✅ Supported | `.go` | `*_test.go` |
| TypeScript | 🔜 Planned | `.ts`, `.tsx` | `*.test.ts`, `*.spec.ts` |

Unsupported languages log a warning and are skipped — they don't fail the index.

## What Gets Indexed

For Go files, the chunker extracts:

- **Package doc comments** — the `// Package foo ...` comment at the top of the file
- **Exported function signatures** — `func FooBar(x int) error` with doc comments (bodies are skipped)
- **Exported method signatures** — `func (s *Server) Handle(r Request) Response` with doc comments
- **Exported type declarations** — `type Config struct { ... }` with doc comments
- **Exported const/var declarations** — `const Version = "1.0.0"` with doc comments
- **Cobra CLI commands** — `Use`, `Short`, `Long` fields from `&cobra.Command{...}`
- **MCP tool registrations** — `Name`, `Description` fields from `mcp.AddTool(...)` calls

Each declaration becomes a separate searchable block in the index.

## What Is NOT Indexed

- Function bodies (implementation details)
- Unexported symbols (internal API)
- Test files (`*_test.go`) — excluded by default
- Files with syntax errors — skipped with a warning
- Vendored dependencies — excluded via `.gitignore` respect
