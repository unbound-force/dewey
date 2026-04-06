# Contract: Code Source

**Package**: `source`  
**File**: `source/code.go`

## Type: CodeSource

```go
// CodeSource implements the Source interface for source code files.
// It uses language-aware chunkers to extract high-signal blocks
// (function signatures, type declarations, doc comments, CLI commands,
// MCP tool registrations) from source code.
//
// Each source file produces one Document where Content is a markdown-
// formatted representation of the extracted declarations. This allows
// the existing indexing pipeline (ParseDocument → PersistBlocks →
// PersistLinks → GenerateEmbeddings) to work unchanged.
type CodeSource struct {
    id             string
    name           string
    basePath       string
    languages      []string       // configured language identifiers
    include        []string       // include path patterns (glob)
    exclude        []string       // exclude path patterns (glob)
    ignorePatterns []string       // extra gitignore-compatible patterns
    recursive      bool           // traverse subdirectories (default: true)
    storedHashes   map[string]string
    lastFetched    time.Time
}
```

## Constructor

```go
// NewCodeSource creates a CodeSource for the given directory path.
// The languages parameter specifies which language chunkers to use.
// Unsupported languages are logged as warnings and skipped (FR-009).
func NewCodeSource(id, name, basePath string, languages []string, opts ...CodeSourceOption) *CodeSource
```

## Options

```go
type CodeSourceOption func(*CodeSource)

func WithCodeIgnorePatterns(patterns []string) CodeSourceOption
func WithCodeInclude(patterns []string) CodeSourceOption
func WithCodeExclude(patterns []string) CodeSourceOption
func WithCodeRecursive(recursive bool) CodeSourceOption
```

## Source Interface Implementation

### List()

```go
func (c *CodeSource) List() ([]Document, error)
```

**Behavior**:
1. Build ignore matcher from `.gitignore` + configured patterns
2. Walk `basePath` recursively (unless `recursive=false`)
3. For each file:
   a. Check extension against registered chunker extensions for configured languages
   b. Skip test files via `chunker.IsTestFile()`
   c. Skip files matching `exclude` patterns
   d. If `include` is non-empty, skip files not matching any `include` pattern
   e. Read file content
   f. Call `chunker.ChunkFile()` — on error, log warning and skip (FR-013)
   g. Format blocks as markdown content
   h. Create `Document` with formatted content and SHA-256 hash
4. Return all documents

**Document Content Format**:
```markdown
# path/to/file.go

## func NewServer

// NewServer creates a new HTTP server with the given configuration.
// It initializes the router and middleware chain.
func NewServer(cfg Config) *Server

## type Config

// Config holds server configuration options.
type Config struct {
    Port    int
    TLSCert string
}
```

### Fetch(id string)

Returns a single document by relative file path. Re-chunks the file.

### Diff()

Compares current file hashes against stored hashes. Returns added/modified/deleted changes. Reuses the same `diffFileChanges` pattern as `DiskSource`.

### Meta()

Returns `SourceMetadata` with `Type: "code"`.

## Invariants

1. `List()` MUST NOT fail due to individual file parse errors (FR-013)
2. `List()` MUST skip test files by default (FR-014)
3. `List()` MUST respect `.gitignore` patterns (FR-007)
4. `List()` MUST log a warning for unsupported languages (FR-009)
5. Documents MUST have unique IDs (relative file path)
6. Document content MUST be valid markdown (for `vault.ParseDocument()` compatibility)
7. `Meta().Type` MUST return `"code"`
