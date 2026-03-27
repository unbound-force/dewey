package source

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// DiskSource implements the Source interface for local Markdown files.
// It scans a directory for .md files and uses content hashing (SHA-256)
// for change detection, matching the VaultStore pattern.
type DiskSource struct {
	id       string
	name     string
	basePath string

	// storedHashes holds content hashes from the last fetch, keyed by
	// relative file path. Used by Diff to detect changes.
	storedHashes map[string]string
	lastFetched  time.Time
}

// NewDiskSource creates a DiskSource for the given directory path.
// Returns a ready-to-use source with an empty stored hashes map.
// Call [DiskSource.SetStoredHashes] before [DiskSource.Diff] to enable
// incremental change detection.
func NewDiskSource(id, name, basePath string) *DiskSource {
	return &DiskSource{
		id:           id,
		name:         name,
		basePath:     basePath,
		storedHashes: make(map[string]string),
	}
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
// skipping hidden directories (e.g., .dewey/, .git/) and unreadable files.
// Updates the source's lastFetched timestamp on success.
// Returns an error if the directory walk itself fails.
func (d *DiskSource) List() ([]Document, error) {
	var docs []Document

	err := filepath.Walk(d.basePath, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return nil // skip errors
		}
		// Skip hidden directories (e.g., .dewey/, .git/).
		if info.IsDir() && strings.HasPrefix(info.Name(), ".") {
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
	currentFiles, err := walkDiskFiles(d.basePath)
	if err != nil {
		return nil, err
	}

	return diffFileChanges(currentFiles, d.storedHashes, d.Fetch), nil
}

// walkDiskFiles walks basePath and returns a map of relPath → SHA-256
// content hash for every .md file found. Hidden directories (names
// starting with ".") are skipped entirely. Unreadable files are
// silently ignored, matching the List behavior.
func walkDiskFiles(basePath string) (map[string]string, error) {
	files := make(map[string]string) // relPath → hash

	err := filepath.Walk(basePath, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if info.IsDir() && strings.HasPrefix(info.Name(), ".") {
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
