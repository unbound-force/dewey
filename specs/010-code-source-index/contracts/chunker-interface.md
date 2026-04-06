# Contract: Chunker Interface

**Package**: `chunker`  
**File**: `chunker/chunker.go`

## Interface: Chunker

```go
// Chunker extracts high-signal blocks from source code files.
// Each implementation handles a specific programming language.
// Implementations must be safe for concurrent use.
type Chunker interface {
    // ChunkFile parses a source file and returns extracted blocks.
    // The path is used for error reporting and context.
    // Returns an error if the file cannot be parsed (e.g., syntax errors).
    // Callers should handle errors gracefully (skip file, continue indexing).
    ChunkFile(path string, content []byte) ([]Block, error)

    // IsTestFile reports whether the given file path is a test file
    // for this language (e.g., *_test.go for Go).
    IsTestFile(path string) bool

    // Extensions returns the file extensions this chunker handles
    // (e.g., [".go"] for Go, [".ts", ".tsx"] for TypeScript).
    Extensions() []string

    // Language returns the language identifier (e.g., "go", "typescript").
    Language() string
}
```

## Type: Block

```go
// Block represents a single extracted declaration from source code.
// Each block becomes a searchable unit in the knowledge graph index.
type Block struct {
    // Heading is the block title (e.g., "func NewServer", "type Config").
    // Used as the markdown heading when formatting for indexing.
    Heading string

    // Content is the full block content including doc comments and signature.
    // For functions: doc comment + signature (no body).
    // For types: doc comment + type declaration.
    // For Cobra commands: Use, Short, Long fields.
    Content string

    // Kind categorizes the block for manifest generation.
    // Values: "package", "func", "type", "const", "var", "command", "tool"
    Kind string
}
```

## Registry Functions

```go
// Register adds a chunker for a language. Panics if the language
// is already registered (programming error, not runtime error).
func Register(language string, c Chunker)

// Get returns the chunker for a language, or nil if not registered.
func Get(language string) Chunker

// Languages returns all registered language names.
func Languages() []string

// ExtensionMap returns a map of file extension → language name
// for all registered chunkers. Used by CodeSource to determine
// which chunker handles each file.
func ExtensionMap() map[string]string
```

## Invariants

1. `ChunkFile` MUST NOT panic on any input — syntax errors return `error`
2. `ChunkFile` MUST return empty slice (not nil) when no declarations are found
3. `Block.Content` MUST include the doc comment (if any) above the declaration
4. `Block.Content` for functions MUST NOT include the function body
5. `IsTestFile` MUST use language-specific conventions (not a generic pattern)
6. `Extensions()` MUST return lowercase extensions with leading dot (e.g., `".go"`)
7. `Register` MUST be called during `init()` — not at runtime
