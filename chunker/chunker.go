// Package chunker provides a pluggable interface for language-aware source
// code parsing. Each language implementation extracts high-signal blocks
// (exported declarations, doc comments, CLI commands, MCP tools) from
// source files, producing structured [Block] values suitable for indexing
// into a knowledge graph.
//
// The package uses an init-time registration pattern: language chunkers
// call [Register] during init() to make themselves available. Consumers
// look up chunkers by language name via [Get] or by file extension via
// [ForExtension].
//
// Design decision: Block is chunker-specific (not types.BlockEntity) to
// avoid coupling the chunker to the indexing pipeline. The code source
// converts Blocks to Documents at the integration boundary.
package chunker

import (
	"sort"
	"sync"
)

// Block represents a single semantic unit extracted from source code.
// Each block becomes a searchable unit in the knowledge graph index.
type Block struct {
	// Heading is the block title used as a markdown heading when formatting
	// for indexing (e.g., "func NewServer", "type Config", "command: serve").
	Heading string

	// Content is the full block content including doc comments and signature.
	// For functions: doc comment + signature (no body).
	// For types: doc comment + type declaration with fields.
	// For Cobra commands: Use, Short, Long fields.
	// For MCP tools: Name, Description fields.
	Content string

	// Kind categorizes the block for manifest generation and filtering.
	// Values: "package", "func", "type", "const", "var", "command", "tool".
	Kind string
}

// Chunker extracts high-signal blocks from source code files.
// Each implementation handles a specific programming language.
// Implementations must be safe for concurrent use.
type Chunker interface {
	// ChunkFile parses the given source file content and returns extracted
	// blocks. The filename is used for error messages and context.
	// Returns an error if the file cannot be parsed (e.g., syntax errors).
	// Callers should handle errors gracefully (skip file, continue indexing).
	ChunkFile(filename string, content []byte) ([]Block, error)

	// IsTestFile reports whether the given filename is a test file
	// for this language (e.g., *_test.go for Go).
	IsTestFile(filename string) bool

	// Extensions returns the file extensions this chunker handles
	// (e.g., [".go"] for Go, [".ts", ".tsx"] for TypeScript).
	// Extensions must be lowercase with a leading dot.
	Extensions() []string

	// Language returns the language identifier (e.g., "go", "typescript").
	Language() string
}

var (
	mu       sync.RWMutex
	registry = make(map[string]Chunker)
	extMap   = make(map[string]Chunker) // extension -> chunker lookup
)

// Register adds a chunker to the global registry, keyed by its Language().
// It also populates the extension lookup map from the chunker's Extensions().
// Panics if a chunker for the same language is already registered — this is
// a programming error (duplicate init registrations), not a runtime error.
//
// Register must be called during init(), not at runtime.
func Register(c Chunker) {
	mu.Lock()
	defer mu.Unlock()

	lang := c.Language()
	if _, exists := registry[lang]; exists {
		panic("chunker: duplicate registration for language " + lang)
	}
	registry[lang] = c
	for _, ext := range c.Extensions() {
		extMap[ext] = c
	}
}

// Get returns the chunker registered for the given language name, and a
// boolean indicating whether it was found. Returns (nil, false) if no
// chunker is registered for the language.
func Get(language string) (Chunker, bool) {
	mu.RLock()
	defer mu.RUnlock()

	c, ok := registry[language]
	return c, ok
}

// ForExtension returns the chunker that handles the given file extension,
// and a boolean indicating whether one was found. The extension must
// include the leading dot (e.g., ".go"). Returns (nil, false) if no
// chunker handles the extension.
func ForExtension(ext string) (Chunker, bool) {
	mu.RLock()
	defer mu.RUnlock()

	c, ok := extMap[ext]
	return c, ok
}

// Languages returns a sorted list of all registered language names.
// The sort order is deterministic (alphabetical) for stable output.
func Languages() []string {
	mu.RLock()
	defer mu.RUnlock()

	langs := make([]string, 0, len(registry))
	for lang := range registry {
		langs = append(langs, lang)
	}
	sort.Strings(langs)
	return langs
}

// ExtensionMap returns a map of file extension to language name for all
// registered chunkers. Used by CodeSource to determine which chunker
// handles each file. The returned map is a copy — callers may modify it
// without affecting the registry.
func ExtensionMap() map[string]string {
	mu.RLock()
	defer mu.RUnlock()

	m := make(map[string]string, len(extMap))
	for ext, c := range extMap {
		m[ext] = c.Language()
	}
	return m
}
