package source

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/unbound-force/dewey/chunker"
	"github.com/unbound-force/dewey/ignore"
)

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
	languages      []string          // configured language identifiers
	include        []string          // include path patterns (glob)
	exclude        []string          // exclude path patterns (glob)
	ignorePatterns []string          // extra gitignore-compatible patterns
	recursive      bool              // traverse subdirectories (default: true)
	storedHashes   map[string]string // relPath → SHA-256 hash for Diff
	lastFetched    time.Time
}

// CodeSourceOption configures optional behavior for a CodeSource.
// Use With* constructors to create options.
type CodeSourceOption func(*CodeSource)

// WithCodeIgnorePatterns returns a CodeSourceOption that sets additional
// gitignore-compatible patterns for the CodeSource. These patterns
// are merged with any .gitignore file found in the source's base
// directory (union merge semantics).
func WithCodeIgnorePatterns(patterns []string) CodeSourceOption {
	return func(c *CodeSource) {
		c.ignorePatterns = patterns
	}
}

// WithCodeInclude returns a CodeSourceOption that sets include path
// patterns. When non-empty, only files matching at least one include
// pattern are processed. Patterns are matched against the relative
// path from basePath using filepath.Match semantics.
func WithCodeInclude(patterns []string) CodeSourceOption {
	return func(c *CodeSource) {
		c.include = patterns
	}
}

// WithCodeExclude returns a CodeSourceOption that sets exclude path
// patterns. Files matching any exclude pattern are skipped. Patterns
// are matched against the relative path from basePath.
func WithCodeExclude(patterns []string) CodeSourceOption {
	return func(c *CodeSource) {
		c.exclude = patterns
	}
}

// WithCodeRecursive returns a CodeSourceOption that controls whether
// subdirectories are traversed during List and Diff walks. When
// false, only files in the base directory are included.
func WithCodeRecursive(recursive bool) CodeSourceOption {
	return func(c *CodeSource) {
		c.recursive = recursive
	}
}

// NewCodeSource creates a CodeSource for the given directory path.
// The languages parameter specifies which language chunkers to use.
// Unsupported languages are logged as warnings and skipped (FR-009).
//
// Options are applied after defaults. The default configuration is
// recursive=true with no extra ignore, include, or exclude patterns.
func NewCodeSource(id, name, basePath string, languages []string, opts ...CodeSourceOption) *CodeSource {
	c := &CodeSource{
		id:           id,
		name:         name,
		basePath:     basePath,
		languages:    languages,
		recursive:    true,
		storedHashes: make(map[string]string),
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// SetStoredHashes sets the previously known content hashes for change
// detection. The hashes map is keyed by relative file path with SHA-256
// hex digest values. Call this before [CodeSource.Diff] to enable
// incremental updates; without stored hashes, Diff reports all files
// as added.
func (c *CodeSource) SetStoredHashes(hashes map[string]string) {
	c.storedHashes = hashes
}

// List returns all source code files in the source directory as Documents,
// each containing markdown-formatted declarations extracted by the
// appropriate language chunker.
//
// Skips test files (FR-014), files with syntax errors (FR-013, logged
// warning), files matching exclude patterns, and files not matching
// include patterns (when include is non-empty). Respects .gitignore
// patterns (FR-007). Logs warnings for unsupported languages (FR-009).
func (c *CodeSource) List() ([]Document, error) {
	// Build the set of valid extensions for configured languages.
	// Unsupported languages are logged as warnings and skipped (FR-009).
	validExts := c.buildExtensionSet()

	matcher, err := ignore.NewMatcher(
		filepath.Join(c.basePath, ".gitignore"),
		c.ignorePatterns,
	)
	if err != nil {
		return nil, fmt.Errorf("build ignore matcher for %q: %w", c.basePath, err)
	}

	var docs []Document

	err = filepath.Walk(c.basePath, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			logger.Debug("skipping path", "path", path, "err", walkErr)
			return nil
		}

		// Use the unified ignore matcher for skip decisions.
		if matcher.ShouldSkip(info.Name(), info.IsDir()) {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		// Non-recursive mode: skip subdirectories (but not basePath itself).
		if !c.recursive && info.IsDir() && path != c.basePath {
			return filepath.SkipDir
		}

		if info.IsDir() {
			return nil
		}

		// Check file extension against registered chunkers for configured languages.
		ext := strings.ToLower(filepath.Ext(info.Name()))
		if !validExts[ext] {
			return nil
		}

		// Look up the chunker for this extension.
		ch, ok := chunker.ForExtension(ext)
		if !ok {
			return nil
		}

		// Skip test files (FR-014).
		if ch.IsTestFile(info.Name()) {
			return nil
		}

		relPath, _ := filepath.Rel(c.basePath, path)
		relPath = filepath.ToSlash(relPath)

		// Apply exclude patterns: skip files matching any exclude pattern.
		if c.matchesAnyPattern(relPath, c.exclude) {
			return nil
		}

		// Apply include patterns: when non-empty, skip files not matching
		// any include pattern.
		if len(c.include) > 0 && !c.matchesAnyPattern(relPath, c.include) {
			return nil
		}

		// Read file content.
		content, readErr := os.ReadFile(path)
		if readErr != nil {
			logger.Debug("skipping unreadable file", "path", path, "err", readErr)
			return nil
		}

		// Run the chunker. On error, log warning and skip (FR-013).
		blocks, chunkErr := ch.ChunkFile(info.Name(), content)
		if chunkErr != nil {
			logger.Warn("skipping file with parse error",
				"path", relPath, "err", chunkErr)
			return nil
		}

		// Skip files that produce no blocks (nothing to index).
		if len(blocks) == 0 {
			return nil
		}

		// Format blocks as markdown content. Each block becomes a
		// heading + content section, producing valid markdown for
		// the existing ParseDocument pipeline.
		mdContent := formatBlocksAsMarkdown(relPath, blocks)

		doc := Document{
			ID:          relPath,
			Title:       relPath,
			Content:     mdContent,
			ContentHash: computeHash(mdContent),
			SourceID:    c.id,
			FetchedAt:   time.Now(),
			Properties: map[string]any{
				"language":    ch.Language(),
				"file_path":   relPath,
				"block_count": len(blocks),
			},
		}
		docs = append(docs, doc)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk code source %q: %w", c.basePath, err)
	}

	c.lastFetched = time.Now()
	return docs, nil
}

// Fetch retrieves a single document by its relative file path (e.g.,
// "cmd/serve.go"). Re-chunks the file and returns the formatted document.
// Returns an error if the file cannot be read or parsed.
func (c *CodeSource) Fetch(id string) (*Document, error) {
	absPath := filepath.Join(c.basePath, filepath.FromSlash(id))
	content, err := os.ReadFile(absPath)
	if err != nil {
		return nil, fmt.Errorf("read file %q: %w", id, err)
	}

	ext := strings.ToLower(filepath.Ext(id))
	ch, ok := chunker.ForExtension(ext)
	if !ok {
		return nil, fmt.Errorf("no chunker for extension %q", ext)
	}

	blocks, err := ch.ChunkFile(filepath.Base(id), content)
	if err != nil {
		return nil, fmt.Errorf("chunk file %q: %w", id, err)
	}

	mdContent := formatBlocksAsMarkdown(id, blocks)

	doc := &Document{
		ID:          id,
		Title:       id,
		Content:     mdContent,
		ContentHash: computeHash(mdContent),
		SourceID:    c.id,
		FetchedAt:   time.Now(),
		Properties: map[string]any{
			"language":    ch.Language(),
			"file_path":   id,
			"block_count": len(blocks),
		},
	}
	return doc, nil
}

// Diff returns changes since the last fetch by comparing current file
// hashes against stored hashes. Uses the same SHA-256 algorithm and
// diffFileChanges helper as DiskSource for consistent change detection.
func (c *CodeSource) Diff() ([]Change, error) {
	// Build the set of valid extensions for configured languages.
	validExts := c.buildExtensionSet()

	matcher, err := ignore.NewMatcher(
		filepath.Join(c.basePath, ".gitignore"),
		c.ignorePatterns,
	)
	if err != nil {
		return nil, fmt.Errorf("build ignore matcher for diff %q: %w", c.basePath, err)
	}

	currentFiles, err := walkCodeFiles(c.basePath, matcher, c.recursive, validExts, c.include, c.exclude)
	if err != nil {
		return nil, err
	}

	return diffFileChanges(currentFiles, c.storedHashes, c.Fetch), nil
}

// Meta returns metadata about this code source, including its ID, type
// ("code"), name, status, and last fetch timestamp.
func (c *CodeSource) Meta() SourceMetadata {
	return SourceMetadata{
		ID:            c.id,
		Type:          "code",
		Name:          c.name,
		Status:        "active",
		LastFetchedAt: c.lastFetched,
	}
}

// buildExtensionSet returns the set of file extensions handled by
// chunkers for the configured languages. Logs a warning for any
// language that has no registered chunker (FR-009).
func (c *CodeSource) buildExtensionSet() map[string]bool {
	exts := make(map[string]bool)
	for _, lang := range c.languages {
		ch, ok := chunker.Get(lang)
		if !ok {
			logger.Warn("unsupported language, no chunker registered",
				"language", lang, "source", c.id)
			continue
		}
		for _, ext := range ch.Extensions() {
			exts[ext] = true
		}
	}
	return exts
}

// matchesAnyPattern checks if the relative path matches any of the
// given glob patterns. Patterns are matched against the path using
// prefix matching (for directory patterns like "cmd/") and
// filepath.Match (for glob patterns).
func (c *CodeSource) matchesAnyPattern(relPath string, patterns []string) bool {
	for _, pat := range patterns {
		// Support directory prefix patterns (e.g., "cmd/", "vendor/").
		if strings.HasSuffix(pat, "/") {
			if strings.HasPrefix(relPath, pat) {
				return true
			}
			continue
		}

		// Try filepath.Match for glob patterns.
		if matched, _ := filepath.Match(pat, relPath); matched {
			return true
		}

		// Also try matching against just the filename for simple patterns.
		if matched, _ := filepath.Match(pat, filepath.Base(relPath)); matched {
			return true
		}

		// Try prefix match for directory-like patterns without trailing slash.
		if strings.HasPrefix(relPath, pat+"/") {
			return true
		}
	}
	return false
}

// walkCodeFiles walks basePath and returns a map of relPath → SHA-256
// content hash for every source code file that passes all filters.
// This is the code source equivalent of walkDiskFiles, adapted for
// language-aware filtering.
func walkCodeFiles(basePath string, matcher *ignore.Matcher, recursive bool, validExts map[string]bool, include, exclude []string) (map[string]string, error) {
	files := make(map[string]string)

	// Create a temporary CodeSource just for pattern matching.
	// Design decision: reuse matchesAnyPattern via a zero-value struct
	// rather than duplicating the logic. The struct fields are only used
	// for pattern matching, not for any state.
	patMatcher := &CodeSource{include: include, exclude: exclude}

	err := filepath.Walk(basePath, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return nil
		}

		if matcher.ShouldSkip(info.Name(), info.IsDir()) {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		if !recursive && info.IsDir() && path != basePath {
			return filepath.SkipDir
		}

		if info.IsDir() {
			return nil
		}

		ext := strings.ToLower(filepath.Ext(info.Name()))
		if !validExts[ext] {
			return nil
		}

		ch, ok := chunker.ForExtension(ext)
		if !ok {
			return nil
		}

		if ch.IsTestFile(info.Name()) {
			return nil
		}

		relPath, _ := filepath.Rel(basePath, path)
		relPath = filepath.ToSlash(relPath)

		if patMatcher.matchesAnyPattern(relPath, exclude) {
			return nil
		}

		if len(include) > 0 && !patMatcher.matchesAnyPattern(relPath, include) {
			return nil
		}

		content, err := os.ReadFile(path)
		if err != nil {
			return nil
		}

		// Chunk the file to produce the same content hash as List.
		blocks, chunkErr := ch.ChunkFile(info.Name(), content)
		if chunkErr != nil {
			return nil
		}

		if len(blocks) == 0 {
			return nil
		}

		mdContent := formatBlocksAsMarkdown(relPath, blocks)
		files[relPath] = computeHash(mdContent)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk code source for diff: %w", err)
	}

	return files, nil
}

// formatBlocksAsMarkdown converts chunker blocks into a markdown document.
// The document starts with a level-1 heading (the file path), followed by
// each block as a level-2 heading and its content. This format is compatible
// with the existing ParseDocument pipeline.
func formatBlocksAsMarkdown(filePath string, blocks []chunker.Block) string {
	var sb strings.Builder
	sb.WriteString("# ")
	sb.WriteString(filePath)
	sb.WriteString("\n\n")

	for _, b := range blocks {
		sb.WriteString("## ")
		sb.WriteString(b.Heading)
		sb.WriteString("\n\n")
		sb.WriteString(b.Content)
		sb.WriteString("\n\n")
	}

	return sb.String()
}
