# Phase 0 Research: Code Source Indexing & Manifest Generation

**Branch**: `010-code-source-index` | **Date**: 2026-04-06

## R1: Go AST Parsing for Code Chunking

**Question**: How to extract high-signal blocks (function signatures, type declarations, doc comments, Cobra commands, MCP tools) from Go source code?

**Finding**: The Go standard library provides `go/parser` and `go/ast` packages that parse Go source files into a complete AST. This is the correct approach — no regex, no CGO, no external dependencies.

Key capabilities:
- `parser.ParseFile(fset, path, content, parser.ParseComments)` — parses a single file with comments
- `ast.File.Decls` — iterates top-level declarations (GenDecl for types/vars/consts, FuncDecl for functions)
- `ast.FuncDecl.Doc` — associated doc comment group
- `ast.GenDecl.Doc` — associated doc comment group for type/var/const blocks
- `ast.File.Doc` — package-level doc comment
- `go/format.Node()` — can format AST nodes back to source (useful for extracting signatures)

For function signatures without bodies: extract `ast.FuncDecl.Type` (params + results) and `ast.FuncDecl.Recv` (receiver), format with `go/format`, and prepend the doc comment. Skip `ast.FuncDecl.Body`.

**Decision**: Use `go/parser` + `go/ast` (stdlib). No external dependencies needed. This satisfies the constitution's CGO prohibition and dependency minimization.

## R2: Cobra Command Detection via AST

**Question**: How to detect Cobra command definitions (`&cobra.Command{Use: "...", Short: "...", Long: "..."}`) in AST?

**Finding**: Cobra commands are typically defined as composite literals of type `*cobra.Command` or `cobra.Command`. In the AST:
1. Look for `ast.CompositeLit` where the type is `cobra.Command` (or `*cobra.Command` via `ast.UnaryExpr`)
2. Within the composite literal, iterate `ast.KeyValueExpr` elements
3. Extract `Use`, `Short`, `Long` fields where the key is an `ast.Ident` and the value is an `ast.BasicLit` (string literal)

Pattern to match:
```go
cmd := &cobra.Command{
    Use:   "name",
    Short: "description",
    Long:  "detailed description",
}
```

Edge cases:
- Commands defined as package-level variables vs. returned from functions — both produce `ast.CompositeLit`
- String concatenation in `Long` fields — only extract `ast.BasicLit` (simple string literals), skip complex expressions
- Dynamic command generation — not detectable via static analysis, documented as a known limitation

**Decision**: Walk AST looking for `ast.CompositeLit` with type matching `cobra.Command`. Extract `Use`/`Short`/`Long` string fields. Non-string values are skipped with a debug log.

## R3: MCP Tool Registration Detection

**Question**: How to detect MCP tool registrations (`mcp.AddTool(srv, &mcp.Tool{Name: "...", Description: "..."}, handler)`) in AST?

**Finding**: MCP tool registrations follow the pattern:
```go
mcp.AddTool(srv, &mcp.Tool{
    Name:        "tool_name",
    Description: "tool description",
}, handler)
```

In the AST:
1. Look for `ast.CallExpr` where the function is `mcp.AddTool` (selector expression)
2. The second argument is typically `&mcp.Tool{...}` — an `ast.UnaryExpr` containing an `ast.CompositeLit`
3. Extract `Name` and `Description` fields from the composite literal

This is structurally identical to Cobra detection — both are composite literal field extraction.

**Decision**: Reuse the same composite literal field extraction logic for both Cobra and MCP tool detection. The chunker walks all `ast.CallExpr` nodes looking for `mcp.AddTool` calls.

## R4: Chunker Interface Design

**Question**: What interface should language chunkers implement?

**Finding**: The chunker needs to:
1. Accept a file path and content bytes
2. Return a list of blocks (each with a title/heading and content)
3. Be registerable by language name
4. Handle errors per-file (skip on syntax error, don't fail the whole index)

The output blocks need to be compatible with the existing `types.BlockEntity` structure used by `vault.PersistBlocks()`. However, the chunker should not depend on `types` directly — it should return its own `Block` type that the `CodeSource` converts to `types.BlockEntity`.

**Decision**: Define `chunker.Chunker` interface with `ChunkFile(path string, content []byte) ([]Block, error)` where `Block` is `{Heading string, Content string}`. The `CodeSource` converts these to `types.BlockEntity` with deterministic UUIDs. A `chunker.Registry` maps language names to implementations.

## R5: Code Source Integration with Existing Pipeline

**Question**: How does the code source fit into the existing indexing pipeline?

**Finding**: The existing pipeline in `indexDocuments()` (cli.go) processes `source.Document` objects:
1. `source.Manager.FetchAll()` → returns `map[string][]source.Document`
2. For each document: `vault.ParseDocument()` → parses markdown into blocks
3. `vault.PersistBlocks()` → stores blocks in SQLite
4. `vault.PersistLinks()` → stores wikilinks
5. `vault.GenerateEmbeddings()` → creates vector embeddings

For code sources, step 2 changes: instead of `vault.ParseDocument()` (markdown parser), the code source produces `source.Document` objects where `Content` is already the chunked output formatted as markdown. Each Go declaration becomes a document (or the code source pre-formats all declarations into a single markdown document per file).

**Better approach**: The `CodeSource.List()` method uses the chunker to produce one `source.Document` per source file, where `Content` is a markdown-formatted representation of the extracted declarations. This way, the existing `vault.ParseDocument()` → `PersistBlocks()` pipeline works unchanged.

**Decision**: `CodeSource.List()` returns `source.Document` objects with markdown-formatted content. Each file produces one document. The chunker output (declarations) is formatted as markdown headings + code blocks. This reuses the entire existing indexing pipeline without modification.

## R6: Manifest Generation Approach

**Question**: How should `dewey manifest` generate the manifest file?

**Finding**: The manifest command:
1. Walks `.go` files in the current directory (using the same ignore infrastructure)
2. Runs the Go chunker on each file
3. Collects all Cobra commands, MCP tools, and exported packages
4. Formats them into a markdown file at `.dewey/manifest.md`

The manifest is a regular markdown file — no special format. It gets indexed by disk sources like any other `.md` file.

Sections:
- `# Project Manifest` — generated header with timestamp
- `## CLI Commands` — table or list of command name, description
- `## MCP Tools` — table or list of tool name, description
- `## Exported Packages` — package path, doc comment summary

**Decision**: Generate `.dewey/manifest.md` as plain markdown. Use the same chunker as the code source. The manifest command is a standalone CLI command, not part of the indexing pipeline.

## R7: File Extension Mapping

**Question**: How to map language names to file extensions?

**Finding**: The registry needs to map:
- `"go"` → `[".go"]` (excluding `_test.go`)
- Future: `"typescript"` → `[".ts", ".tsx"]`
- Future: `"python"` → `[".py"]`

The mapping lives in the registry alongside the chunker implementation. When `CodeSource` walks files, it checks the extension against the registered languages' extensions.

**Decision**: Each registered chunker declares its file extensions. The registry provides a `Extensions(language string) []string` method. The `CodeSource` uses this to filter files during walk.

## R8: Test File Exclusion

**Question**: How to exclude test files from indexing?

**Finding**: Per FR-014, test files must be excluded by default. For Go, this means `*_test.go` files. Each language has its own test file convention.

The exclusion should be in the chunker, not the source — the chunker knows the language-specific test file patterns. The `Chunker` interface can include an `IsTestFile(path string) bool` method, or the `ChunkFile` method can return empty blocks for test files.

**Decision**: The `Chunker` interface includes `IsTestFile(path string) bool`. The `CodeSource` calls this before `ChunkFile` and skips test files. This keeps test-file knowledge in the language-specific chunker.
