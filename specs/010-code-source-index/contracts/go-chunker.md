# Contract: Go Language Chunker

**Package**: `chunker`  
**File**: `chunker/go_chunker.go`

## Type: GoChunker

```go
// GoChunker implements the Chunker interface for Go source files.
// It uses go/parser and go/ast from the standard library to extract
// exported declarations, doc comments, Cobra commands, and MCP tools.
//
// Design decision: Uses go/parser (stdlib) instead of regex for correct
// AST walking, multi-line string handling, and doc comment association.
// No CGO dependency — satisfies constitution's dependency minimization.
type GoChunker struct{}
```

## ChunkFile Behavior

Given a Go source file, `ChunkFile` extracts the following block types:

### 1. Package Doc Comment (Kind: "package")

```go
// Input:
// Package server provides HTTP server functionality.
package server

// Output Block:
Block{
    Heading: "package server",
    Content: "// Package server provides HTTP server functionality.\npackage server",
    Kind:    "package",
}
```

### 2. Exported Function Signatures (Kind: "func")

```go
// Input:
// NewServer creates a new HTTP server.
func NewServer(cfg Config) *Server {
    // body omitted
}

// Output Block:
Block{
    Heading: "func NewServer",
    Content: "// NewServer creates a new HTTP server.\nfunc NewServer(cfg Config) *Server",
    Kind:    "func",
}
```

### 3. Exported Method Signatures (Kind: "func")

```go
// Input:
// Handle processes an incoming request.
func (s *Server) Handle(r Request) Response {
    // body omitted
}

// Output Block:
Block{
    Heading: "func (*Server) Handle",
    Content: "// Handle processes an incoming request.\nfunc (s *Server) Handle(r Request) Response",
    Kind:    "func",
}
```

### 4. Exported Type Declarations (Kind: "type")

```go
// Input:
// Config holds server configuration.
type Config struct {
    Port int
    Host string
}

// Output Block:
Block{
    Heading: "type Config",
    Content: "// Config holds server configuration.\ntype Config struct {\n\tPort int\n\tHost string\n}",
    Kind:    "type",
}
```

### 5. Exported Const/Var Declarations (Kind: "const" or "var")

```go
// Input:
// Version is the current server version.
const Version = "1.0.0"

// Output Block:
Block{
    Heading: "const Version",
    Content: "// Version is the current server version.\nconst Version = \"1.0.0\"",
    Kind:    "const",
}
```

### 6. Cobra Command Definitions (Kind: "command")

```go
// Input:
cmd := &cobra.Command{
    Use:   "serve",
    Short: "Start the MCP server",
    Long:  "Start the MCP server with stdio or HTTP transport.",
}

// Output Block:
Block{
    Heading: "command: serve",
    Content: "CLI Command: serve\nShort: Start the MCP server\nLong: Start the MCP server with stdio or HTTP transport.",
    Kind:    "command",
}
```

### 7. MCP Tool Registrations (Kind: "tool")

```go
// Input:
mcp.AddTool(srv, &mcp.Tool{
    Name:        "get_page",
    Description: "Get a page with its block tree",
}, handler)

// Output Block:
Block{
    Heading: "tool: get_page",
    Content: "MCP Tool: get_page\nDescription: Get a page with its block tree",
    Kind:    "tool",
}
```

## IsTestFile Behavior

```go
func (g *GoChunker) IsTestFile(path string) bool
```

Returns `true` if `filepath.Base(path)` matches `*_test.go`.

## Extensions

Returns `[]string{".go"}`.

## Language

Returns `"go"`.

## Error Handling

- Syntax errors in Go files: return `error` (caller skips file)
- Files with no exported declarations: return empty `[]Block{}`
- Unexported symbols: silently skipped (not included in output)
- Non-string Cobra/MCP fields (e.g., variable references): skipped with debug log

## Invariants

1. Function bodies MUST NOT appear in output (FR-005)
2. Only exported symbols (uppercase first letter) are included
3. Doc comments MUST be associated with their declaration (not free-floating comments)
4. Cobra detection MUST handle both `cobra.Command` and `*cobra.Command` composite literals
5. MCP detection MUST handle the `mcp.AddTool(srv, &mcp.Tool{...}, handler)` pattern
6. `ChunkFile` MUST NOT panic on any valid or invalid Go source input
