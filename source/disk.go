package source

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/unbound-force/dewey/ignore"
)

// DiskSource implements the Source interface for local Markdown files.
// It scans a directory for .md files and uses content hashing (SHA-256)
// for change detection, matching the VaultStore pattern.
type DiskSource struct {
	id       string
	name     string
	basePath string

	// ignorePatterns holds extra gitignore-compatible patterns from
	// sources.yaml configuration. These are merged with .gitignore
	// patterns via the ignore.Matcher (union merge semantics).
	ignorePatterns []string

	// recursive controls whether subdirectories are traversed during
	// List and Diff walks. Defaults to true.
	recursive bool

	// storedHashes holds content hashes from the last fetch, keyed by
	// relative file path. Used by Diff to detect changes.
	storedHashes map[string]string
	lastFetched  time.Time
}

// DiskSourceOption configures optional behavior for a DiskSource.
// Use With* constructors to create options.
type DiskSourceOption func(*DiskSource)

// WithIgnorePatterns returns a DiskSourceOption that sets additional
// gitignore-compatible patterns for the DiskSource. These patterns
// are merged with any .gitignore file found in the source's base
// directory (union merge semantics per FR-005).
func WithIgnorePatterns(patterns []string) DiskSourceOption {
	return func(d *DiskSource) {
		d.ignorePatterns = patterns
	}
}

// WithRecursive returns a DiskSourceOption that controls whether
// subdirectories are traversed during List and Diff walks. When
// false, only files in the base directory are included.
func WithRecursive(recursive bool) DiskSourceOption {
	return func(d *DiskSource) {
		d.recursive = recursive
	}
}

// NewDiskSource creates a DiskSource for the given directory path.
// Returns a ready-to-use source with an empty stored hashes map.
// Call [DiskSource.SetStoredHashes] before [DiskSource.Diff] to enable
// incremental change detection.
//
// Options are applied after defaults. The default configuration is
// recursive=true with no extra ignore patterns. The variadic opts
// parameter ensures backward compatibility — existing callers that
// pass zero options continue to work unchanged.
func NewDiskSource(id, name, basePath string, opts ...DiskSourceOption) *DiskSource {
	d := &DiskSource{
		id:           id,
		name:         name,
		basePath:     basePath,
		recursive:    true,
		storedHashes: make(map[string]string),
	}
	for _, opt := range opts {
		opt(d)
	}
	return d
}

// SetStoredHashes sets the previously known content hashes for change
// detection. The hashes map is keyed by relative file path with SHA-256
// hex digest values. Call this before [DiskSource.Diff] to enable
// incremental updates; without stored hashes, Diff reports all files
// as added.
func (d *DiskSource) SetStoredHashes(hashes map[string]string) {
	d.storedHashes = hashes
}

// List returns all .md files in the source directory as Documents,
// skipping ignored entries (hidden directories, .gitignore patterns,
// and configured ignore patterns) and unreadable files.
// Updates the source's lastFetched timestamp on success.
// Returns an error if the directory walk itself fails or the ignore
// matcher cannot be constructed.
func (d *DiskSource) List() ([]Document, error) {
	matcher, err := ignore.NewMatcher(
		filepath.Join(d.basePath, ".gitignore"),
		d.ignorePatterns,
	)
	if err != nil {
		return nil, fmt.Errorf("build ignore matcher for %q: %w", d.basePath, err)
	}

	var docs []Document

	err = filepath.Walk(d.basePath, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return nil // skip errors
		}

		// Use the unified ignore matcher for skip decisions.
		// This replaces the previous inline strings.HasPrefix(name, ".")
		// check with the full gitignore-compatible matcher, supporting
		// .gitignore patterns, extra patterns, and hidden-dir baseline.
		if matcher.ShouldSkip(info.Name(), info.IsDir()) {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		// Non-recursive mode: skip subdirectories (but not basePath itself).
		if !d.recursive && info.IsDir() && path != d.basePath {
			return filepath.SkipDir
		}

		if info.IsDir() || !strings.HasSuffix(info.Name(), ".md") {
			return nil
		}

		content, err := os.ReadFile(path)
		if err != nil {
			return nil // skip unreadable files
		}

		relPath, _ := filepath.Rel(d.basePath, path)
		relPath = filepath.ToSlash(relPath)
		pageName := strings.TrimSuffix(relPath, ".md")

		doc := Document{
			ID:          relPath,
			Title:       pageName,
			Content:     string(content),
			ContentHash: computeHash(string(content)),
			SourceID:    d.id,
			FetchedAt:   time.Now(),
		}
		docs = append(docs, doc)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk disk source %q: %w", d.basePath, err)
	}

	d.lastFetched = time.Now()
	return docs, nil
}

// Fetch retrieves a single document by its relative file path (e.g.,
// "subfolder/page.md"). Returns the document with computed SHA-256
// content hash. Returns an error if the file cannot be read.
func (d *DiskSource) Fetch(id string) (*Document, error) {
	absPath := filepath.Join(d.basePath, filepath.FromSlash(id))
	content, err := os.ReadFile(absPath)
	if err != nil {
		return nil, fmt.Errorf("read file %q: %w", id, err)
	}

	pageName := strings.TrimSuffix(id, ".md")
	doc := &Document{
		ID:          id,
		Title:       pageName,
		Content:     string(content),
		ContentHash: computeHash(string(content)),
		SourceID:    d.id,
		FetchedAt:   time.Now(),
	}
	return doc, nil
}

// Diff returns changes since the last fetch by comparing current file
// hashes against stored hashes. Uses the same SHA-256 algorithm as VaultStore.
// Returns a slice of changes categorized as [ChangeAdded], [ChangeModified],
// or [ChangeDeleted]. Returns an error if the directory walk fails.
//
// Decomposed into walkDiskFiles (directory scan) and diffFileChanges
// (hash comparison) to keep each function under cyclomatic complexity 10.
func (d *DiskSource) Diff() ([]Change, error) {
	matcher, err := ignore.NewMatcher(
		filepath.Join(d.basePath, ".gitignore"),
		d.ignorePatterns,
	)
	if err != nil {
		return nil, fmt.Errorf("build ignore matcher for diff %q: %w", d.basePath, err)
	}

	currentFiles, err := walkDiskFiles(d.basePath, matcher, d.recursive)
	if err != nil {
		return nil, err
	}

	return diffFileChanges(currentFiles, d.storedHashes, d.Fetch), nil
}

// walkDiskFiles walks basePath and returns a map of relPath → SHA-256
// content hash for every .md file found. The provided matcher determines
// which entries to skip (hidden directories, .gitignore patterns, extra
// patterns). When recursive is false, subdirectories are not traversed.
// Unreadable files are silently ignored, matching the List behavior.
func walkDiskFiles(basePath string, matcher *ignore.Matcher, recursive bool) (map[string]string, error) {
	files := make(map[string]string) // relPath → hash

	err := filepath.Walk(basePath, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
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
		if !recursive && info.IsDir() && path != basePath {
			return filepath.SkipDir
		}

		if info.IsDir() || !strings.HasSuffix(info.Name(), ".md") {
			return nil
		}

		content, err := os.ReadFile(path)
		if err != nil {
			return nil
		}

		relPath, _ := filepath.Rel(basePath, path)
		relPath = filepath.ToSlash(relPath)
		files[relPath] = computeHash(string(content))
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk disk source for diff: %w", err)
	}

	return files, nil
}

// diffFileChanges compares currentFiles against storedHashes and returns
// a slice of changes. New files (in current but not stored) are
// ChangeAdded; files with different hashes are ChangeModified; files
// in stored but not current are ChangeDeleted. For added and modified
// files, fetcher is called to retrieve the full Document; fetch errors
// cause the file to be silently skipped.
func diffFileChanges(currentFiles, storedHashes map[string]string, fetcher func(string) (*Document, error)) []Change {
	var changes []Change

	// Detect new and modified files.
	for relPath, currentHash := range currentFiles {
		storedHash, exists := storedHashes[relPath]
		if !exists {
			doc, err := fetcher(relPath)
			if err != nil {
				continue
			}
			changes = append(changes, Change{
				Type:     ChangeAdded,
				Document: doc,
				ID:       relPath,
			})
		} else if storedHash != currentHash {
			doc, err := fetcher(relPath)
			if err != nil {
				continue
			}
			changes = append(changes, Change{
				Type:     ChangeModified,
				Document: doc,
				ID:       relPath,
			})
		}
	}

	// Detect deleted files.
	for relPath := range storedHashes {
		if _, exists := currentFiles[relPath]; !exists {
			changes = append(changes, Change{
				Type: ChangeDeleted,
				ID:   relPath,
			})
		}
	}

	return changes
}

// Meta returns metadata about this disk source, including its ID, type
// ("disk"), name, status, and last fetch timestamp.
func (d *DiskSource) Meta() SourceMetadata {
	return SourceMetadata{
		ID:            d.id,
		Type:          "disk",
		Name:          d.name,
		Status:        "active",
		LastFetchedAt: d.lastFetched,
	}
}

// computeHash generates a SHA-256 hex digest. Same algorithm as VaultStore
// to ensure consistent change detection across the codebase.
func computeHash(content string) string {
	h := sha256.Sum256([]byte(content))
	return fmt.Sprintf("%x", h)
}
