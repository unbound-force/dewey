// Package source defines the pluggable content source interface for Dewey.
// Sources provide documents from various origins (local disk, GitHub, web crawl)
// that are indexed into the knowledge graph.
package source

import (
	"os"
	"time"

	"github.com/charmbracelet/log"
)

// logger is the package-level structured logger shared across all source files.
// Consolidates the previously separate githubLogger, webLogger, and managerLogger
// into a single declaration to reduce duplication.
var logger = log.NewWithOptions(os.Stderr, log.Options{
	Prefix: "dewey",
})

// SetLogLevel sets the logging level for the source package.
// Use log.DebugLevel for verbose output during diagnostics.
func SetLogLevel(level log.Level) {
	logger.SetLevel(level)
}

// Source represents a pluggable content origin. Implementations fetch documents
// from a specific backend (disk, GitHub API, web crawl) and support incremental
// updates via the Diff method.
type Source interface {
	// List returns all documents available from this source.
	List() ([]Document, error)

	// Fetch retrieves a single document by its source-specific identifier.
	Fetch(id string) (*Document, error)

	// Diff returns changes since the last fetch, enabling incremental indexing.
	// Returns nil if the source does not support incremental updates.
	Diff() ([]Change, error)

	// Meta returns metadata about this source (type, name, status).
	Meta() SourceMetadata
}

// Document represents a content item fetched from a source. Documents are
// transient — they are converted to Pages, Blocks, and Links during indexing
// and are not persisted directly.
type Document struct {
	// ID is the source-specific document identifier (e.g., file path, issue number).
	ID string

	// Title is the human-readable document title.
	Title string

	// Content is the raw text content of the document.
	Content string

	// ContentHash is a hash of the content for change detection.
	ContentHash string

	// SourceID identifies which source produced this document.
	SourceID string

	// OriginURL is the original URL for external sources (nil for disk).
	OriginURL string

	// FetchedAt is when this document was last fetched.
	FetchedAt time.Time

	// Properties holds source-specific metadata (e.g., GitHub labels, web crawl depth).
	Properties map[string]any
}

// Change represents a modification detected by a source's Diff method.
type Change struct {
	// Type indicates the kind of change.
	Type ChangeType

	// Document is the changed document (nil for deletions).
	Document *Document

	// ID is the document identifier (always set, even for deletions).
	ID string
}

// ChangeType enumerates the kinds of changes a source can report.
type ChangeType string

const (
	// ChangeAdded indicates a new document was added.
	ChangeAdded ChangeType = "added"

	// ChangeModified indicates an existing document was modified.
	ChangeModified ChangeType = "modified"

	// ChangeDeleted indicates a document was removed.
	ChangeDeleted ChangeType = "deleted"
)

// SourceMetadata describes a content source's identity and status.
type SourceMetadata struct {
	// ID is the unique source identifier (e.g., "disk-local", "github-gaze").
	ID string

	// Type is the source type (e.g., "disk", "github", "web").
	Type string

	// Name is the human-readable source name.
	Name string

	// Status is the current source status ("active", "error", "disabled").
	Status string

	// ErrorMessage contains the last error if Status is "error".
	ErrorMessage string

	// LastFetchedAt is when the source was last successfully fetched.
	LastFetchedAt time.Time

	// RefreshInterval is how often the source should be re-fetched.
	RefreshInterval string
}
