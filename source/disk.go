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
func NewDiskSource(id, name, basePath string) *DiskSource {
	return &DiskSource{
		id:           id,
		name:         name,
		basePath:     basePath,
		storedHashes: make(map[string]string),
	}
}

// SetStoredHashes sets the previously known content hashes for change detection.
// Call this before Diff() to enable incremental updates.
func (d *DiskSource) SetStoredHashes(hashes map[string]string) {
	d.storedHashes = hashes
}

// List returns all .md files in the source directory as Documents.
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

// Fetch retrieves a single document by its relative file path.
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
func (d *DiskSource) Diff() ([]Change, error) {
	currentFiles := make(map[string]string) // relPath → hash

	err := filepath.Walk(d.basePath, func(path string, info os.FileInfo, walkErr error) error {
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

		relPath, _ := filepath.Rel(d.basePath, path)
		relPath = filepath.ToSlash(relPath)
		currentFiles[relPath] = computeHash(string(content))
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk disk source for diff: %w", err)
	}

	var changes []Change

	// Detect new and modified files.
	for relPath, currentHash := range currentFiles {
		storedHash, exists := d.storedHashes[relPath]
		if !exists {
			doc, err := d.Fetch(relPath)
			if err != nil {
				continue
			}
			changes = append(changes, Change{
				Type:     ChangeAdded,
				Document: doc,
				ID:       relPath,
			})
		} else if storedHash != currentHash {
			doc, err := d.Fetch(relPath)
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
	for relPath := range d.storedHashes {
		if _, exists := currentFiles[relPath]; !exists {
			changes = append(changes, Change{
				Type: ChangeDeleted,
				ID:   relPath,
			})
		}
	}

	return changes, nil
}

// Meta returns metadata about this disk source.
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
