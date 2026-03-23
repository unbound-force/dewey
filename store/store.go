// Package store provides SQLite-backed persistence for the Dewey knowledge graph.
// It stores pages, blocks, links, embeddings, and index metadata in a single
// .dewey/graph.db file using modernc.org/sqlite (pure Go, no CGO).
package store

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"

	_ "modernc.org/sqlite" // Pure-Go SQLite driver registration.
)

// Store wraps a SQLite database connection for knowledge graph persistence.
// It manages pages, blocks, links, embeddings, and index metadata.
// File-level locking prevents concurrent write corruption (T059).
type Store struct {
	db       *sql.DB
	path     string
	lockFile *os.File // File lock for .dewey/ directory (nil for :memory:).
}

// New opens (or creates) a SQLite database at the given path and applies
// schema migrations. Pass an empty string or ":memory:" for an in-memory
// database (useful for testing).
//
// The database is configured with:
//   - WAL journal mode for concurrent read access
//   - Foreign key enforcement
//   - Busy timeout of 5 seconds
func New(path string) (*Store, error) {
	if path == "" {
		path = ":memory:"
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	// SQLite requires single-connection mode to ensure per-connection
	// pragmas (foreign_keys, busy_timeout) apply to all queries.
	// Without this, database/sql may open additional connections that
	// skip pragma initialization.
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(0)

	// Configure SQLite pragmas for performance and correctness.
	pragmas := []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA foreign_keys=ON",
		"PRAGMA busy_timeout=5000",
	}
	for _, p := range pragmas {
		if _, err := db.Exec(p); err != nil {
			_ = db.Close()
			return nil, fmt.Errorf("set pragma %q: %w", p, err)
		}
	}

	s := &Store{db: db, path: path}

	// Acquire file lock for non-memory databases (T059).
	// Prevents concurrent write corruption from multiple Dewey processes.
	if path != ":memory:" {
		lockPath := filepath.Join(filepath.Dir(path), ".dewey.lock")
		lockFile, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o644)
		if err != nil {
			_ = db.Close()
			return nil, fmt.Errorf("create lock file: %w", err)
		}
		// Acquire exclusive, non-blocking lock to prevent concurrent write corruption (T059).
		if err := syscall.Flock(int(lockFile.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
			_ = lockFile.Close()
			_ = db.Close()
			return nil, fmt.Errorf("another Dewey process is using this database: %w", err)
		}
		s.lockFile = lockFile
	}

	if err := s.migrate(); err != nil {
		_ = s.Close()
		return nil, fmt.Errorf("migrate schema: %w", err)
	}

	return s, nil
}

// Close closes the underlying database connection and releases the file lock.
func (s *Store) Close() error {
	if s.lockFile != nil {
		// Release the advisory lock before closing the file descriptor.
		_ = syscall.Flock(int(s.lockFile.Fd()), syscall.LOCK_UN)
		_ = s.lockFile.Close()
	}
	return s.db.Close()
}

// DB returns the underlying *sql.DB for advanced queries.
// Prefer using Store methods for standard operations.
func (s *Store) DB() *sql.DB {
	return s.db
}

// Page represents a document in the knowledge graph.
type Page struct {
	Name         string
	OriginalName string
	SourceID     string
	SourceDocID  string
	Properties   string // JSON
	ContentHash  string
	IsJournal    bool
	CreatedAt    int64
	UpdatedAt    int64
}

// Block represents a heading-delimited section within a page.
type Block struct {
	UUID         string
	PageName     string
	ParentUUID   sql.NullString
	Content      string
	HeadingLevel int
	Position     int
}

// Link represents a directed connection between two pages.
type Link struct {
	FromPage  string
	ToPage    string
	BlockUUID string
}

// InsertPage inserts a new page into the store. Uses parameterized queries
// to prevent SQL injection (FR-028).
func (s *Store) InsertPage(p *Page) error {
	now := time.Now().UnixMilli()
	if p.CreatedAt == 0 {
		p.CreatedAt = now
	}
	if p.UpdatedAt == 0 {
		p.UpdatedAt = now
	}

	_, err := s.db.Exec(`
		INSERT INTO pages (name, original_name, source_id, source_doc_id, properties, content_hash, is_journal, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		p.Name, p.OriginalName, p.SourceID, p.SourceDocID,
		p.Properties, p.ContentHash, boolToInt(p.IsJournal),
		p.CreatedAt, p.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("insert page %q: %w", p.Name, err)
	}
	return nil
}

// GetPage retrieves a page by name. Returns nil if not found.
func (s *Store) GetPage(name string) (*Page, error) {
	p := &Page{}
	var isJournal int
	var sourceDocID, properties, contentHash sql.NullString

	err := s.db.QueryRow(`
		SELECT name, original_name, source_id, source_doc_id, properties, content_hash, is_journal, created_at, updated_at
		FROM pages WHERE name = ?`, name).Scan(
		&p.Name, &p.OriginalName, &p.SourceID, &sourceDocID,
		&properties, &contentHash, &isJournal,
		&p.CreatedAt, &p.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get page %q: %w", name, err)
	}

	p.SourceDocID = sourceDocID.String
	p.Properties = properties.String
	p.ContentHash = contentHash.String
	p.IsJournal = isJournal != 0
	return p, nil
}

// ListPages returns all pages in the store.
func (s *Store) ListPages() ([]*Page, error) {
	rows, err := s.db.Query(`
		SELECT name, original_name, source_id, source_doc_id, properties, content_hash, is_journal, created_at, updated_at
		FROM pages ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("list pages: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var pages []*Page
	for rows.Next() {
		p := &Page{}
		var isJournal int
		var sourceDocID, properties, contentHash sql.NullString

		if err := rows.Scan(
			&p.Name, &p.OriginalName, &p.SourceID, &sourceDocID,
			&properties, &contentHash, &isJournal,
			&p.CreatedAt, &p.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan page: %w", err)
		}

		p.SourceDocID = sourceDocID.String
		p.Properties = properties.String
		p.ContentHash = contentHash.String
		p.IsJournal = isJournal != 0
		pages = append(pages, p)
	}
	return pages, rows.Err()
}

// UpdatePage updates an existing page's mutable fields. The content_hash
// comparison enables incremental indexing — only re-index when content changes.
func (s *Store) UpdatePage(p *Page) error {
	p.UpdatedAt = time.Now().UnixMilli()

	result, err := s.db.Exec(`
		UPDATE pages SET original_name = ?, source_id = ?, source_doc_id = ?,
		properties = ?, content_hash = ?, is_journal = ?, updated_at = ?
		WHERE name = ?`,
		p.OriginalName, p.SourceID, p.SourceDocID,
		p.Properties, p.ContentHash, boolToInt(p.IsJournal),
		p.UpdatedAt, p.Name,
	)
	if err != nil {
		return fmt.Errorf("update page %q: %w", p.Name, err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("check update result: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("page not found: %s", p.Name)
	}
	return nil
}

// DeletePage removes a page and its associated blocks and links (via CASCADE).
func (s *Store) DeletePage(name string) error {
	result, err := s.db.Exec(`DELETE FROM pages WHERE name = ?`, name)
	if err != nil {
		return fmt.Errorf("delete page %q: %w", name, err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("check delete result: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("page not found: %s", name)
	}
	return nil
}

// InsertBlock inserts a new block into the store.
func (s *Store) InsertBlock(b *Block) error {
	_, err := s.db.Exec(`
		INSERT INTO blocks (uuid, page_name, parent_uuid, content, heading_level, position)
		VALUES (?, ?, ?, ?, ?, ?)`,
		b.UUID, b.PageName, b.ParentUUID,
		b.Content, b.HeadingLevel, b.Position,
	)
	if err != nil {
		return fmt.Errorf("insert block %q: %w", b.UUID, err)
	}
	return nil
}

// GetBlock retrieves a block by UUID. Returns nil if not found.
func (s *Store) GetBlock(uuid string) (*Block, error) {
	b := &Block{}
	err := s.db.QueryRow(`
		SELECT uuid, page_name, parent_uuid, content, heading_level, position
		FROM blocks WHERE uuid = ?`, uuid).Scan(
		&b.UUID, &b.PageName, &b.ParentUUID,
		&b.Content, &b.HeadingLevel, &b.Position,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get block %q: %w", uuid, err)
	}
	return b, nil
}

// GetBlocksByPage returns all blocks for a given page, ordered by position.
func (s *Store) GetBlocksByPage(pageName string) ([]*Block, error) {
	rows, err := s.db.Query(`
		SELECT uuid, page_name, parent_uuid, content, heading_level, position
		FROM blocks WHERE page_name = ? ORDER BY position`, pageName)
	if err != nil {
		return nil, fmt.Errorf("get blocks for page %q: %w", pageName, err)
	}
	defer func() { _ = rows.Close() }()

	var blocks []*Block
	for rows.Next() {
		b := &Block{}
		if err := rows.Scan(
			&b.UUID, &b.PageName, &b.ParentUUID,
			&b.Content, &b.HeadingLevel, &b.Position,
		); err != nil {
			return nil, fmt.Errorf("scan block: %w", err)
		}
		blocks = append(blocks, b)
	}
	return blocks, rows.Err()
}

// DeleteBlocksByPage removes all blocks for a given page.
func (s *Store) DeleteBlocksByPage(pageName string) error {
	_, err := s.db.Exec(`DELETE FROM blocks WHERE page_name = ?`, pageName)
	if err != nil {
		return fmt.Errorf("delete blocks for page %q: %w", pageName, err)
	}
	return nil
}

// InsertLink inserts a directed link between two pages.
func (s *Store) InsertLink(l *Link) error {
	_, err := s.db.Exec(`
		INSERT OR IGNORE INTO links (from_page, to_page, block_uuid)
		VALUES (?, ?, ?)`,
		l.FromPage, l.ToPage, l.BlockUUID,
	)
	if err != nil {
		return fmt.Errorf("insert link %q -> %q: %w", l.FromPage, l.ToPage, err)
	}
	return nil
}

// GetForwardLinks returns all pages that the given page links to.
func (s *Store) GetForwardLinks(pageName string) ([]*Link, error) {
	rows, err := s.db.Query(`
		SELECT from_page, to_page, block_uuid
		FROM links WHERE from_page = ?`, pageName)
	if err != nil {
		return nil, fmt.Errorf("get forward links for %q: %w", pageName, err)
	}
	defer func() { _ = rows.Close() }()

	var links []*Link
	for rows.Next() {
		l := &Link{}
		if err := rows.Scan(&l.FromPage, &l.ToPage, &l.BlockUUID); err != nil {
			return nil, fmt.Errorf("scan link: %w", err)
		}
		links = append(links, l)
	}
	return links, rows.Err()
}

// GetBackwardLinks returns all pages that link to the given page.
func (s *Store) GetBackwardLinks(pageName string) ([]*Link, error) {
	rows, err := s.db.Query(`
		SELECT from_page, to_page, block_uuid
		FROM links WHERE to_page = ?`, pageName)
	if err != nil {
		return nil, fmt.Errorf("get backward links for %q: %w", pageName, err)
	}
	defer func() { _ = rows.Close() }()

	var links []*Link
	for rows.Next() {
		l := &Link{}
		if err := rows.Scan(&l.FromPage, &l.ToPage, &l.BlockUUID); err != nil {
			return nil, fmt.Errorf("scan link: %w", err)
		}
		links = append(links, l)
	}
	return links, rows.Err()
}

// DeleteLinksByPage removes all links originating from the given page.
func (s *Store) DeleteLinksByPage(pageName string) error {
	_, err := s.db.Exec(`DELETE FROM links WHERE from_page = ?`, pageName)
	if err != nil {
		return fmt.Errorf("delete links for page %q: %w", pageName, err)
	}
	return nil
}

// GetMeta retrieves a metadata value by key. Returns empty string if not found.
func (s *Store) GetMeta(key string) (string, error) {
	var value string
	err := s.db.QueryRow(`SELECT value FROM metadata WHERE key = ?`, key).Scan(&value)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("get metadata %q: %w", key, err)
	}
	return value, nil
}

// SetMeta sets a metadata key-value pair, inserting or updating as needed.
func (s *Store) SetMeta(key, value string) error {
	_, err := s.db.Exec(`
		INSERT INTO metadata (key, value) VALUES (?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value`,
		key, value,
	)
	if err != nil {
		return fmt.Errorf("set metadata %q: %w", key, err)
	}
	return nil
}

// --- Source operations (T046) ---

// SourceRecord represents a content source in the store.
type SourceRecord struct {
	ID              string
	Type            string
	Name            string
	Config          string // JSON
	RefreshInterval string
	LastFetchedAt   int64
	Status          string
	ErrorMessage    string
}

// InsertSource inserts a new source into the store. Uses parameterized
// queries to prevent SQL injection (FR-028).
func (s *Store) InsertSource(src *SourceRecord) error {
	_, err := s.db.Exec(`
		INSERT INTO sources (id, type, name, config, refresh_interval, last_fetched_at, status, error_message)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		src.ID, src.Type, src.Name, src.Config,
		src.RefreshInterval, src.LastFetchedAt,
		src.Status, src.ErrorMessage,
	)
	if err != nil {
		return fmt.Errorf("insert source %q: %w", src.ID, err)
	}
	return nil
}

// GetSource retrieves a source by ID. Returns nil if not found.
func (s *Store) GetSource(id string) (*SourceRecord, error) {
	src := &SourceRecord{}
	var config, refreshInterval, errorMessage sql.NullString
	var lastFetchedAt sql.NullInt64

	err := s.db.QueryRow(`
		SELECT id, type, name, config, refresh_interval, last_fetched_at, status, error_message
		FROM sources WHERE id = ?`, id).Scan(
		&src.ID, &src.Type, &src.Name, &config,
		&refreshInterval, &lastFetchedAt,
		&src.Status, &errorMessage,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get source %q: %w", id, err)
	}

	src.Config = config.String
	src.RefreshInterval = refreshInterval.String
	src.LastFetchedAt = lastFetchedAt.Int64
	src.ErrorMessage = errorMessage.String
	return src, nil
}

// ListSources returns all sources in the store.
func (s *Store) ListSources() ([]*SourceRecord, error) {
	rows, err := s.db.Query(`
		SELECT id, type, name, config, refresh_interval, last_fetched_at, status, error_message
		FROM sources ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("list sources: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var sources []*SourceRecord
	for rows.Next() {
		src := &SourceRecord{}
		var config, refreshInterval, errorMessage sql.NullString
		var lastFetchedAt sql.NullInt64

		if err := rows.Scan(
			&src.ID, &src.Type, &src.Name, &config,
			&refreshInterval, &lastFetchedAt,
			&src.Status, &errorMessage,
		); err != nil {
			return nil, fmt.Errorf("scan source: %w", err)
		}

		src.Config = config.String
		src.RefreshInterval = refreshInterval.String
		src.LastFetchedAt = lastFetchedAt.Int64
		src.ErrorMessage = errorMessage.String
		sources = append(sources, src)
	}
	return sources, rows.Err()
}

// UpdateSourceStatus updates a source's status and error message.
func (s *Store) UpdateSourceStatus(id, status, errorMessage string) error {
	result, err := s.db.Exec(`
		UPDATE sources SET status = ?, error_message = ?
		WHERE id = ?`,
		status, errorMessage, id,
	)
	if err != nil {
		return fmt.Errorf("update source status %q: %w", id, err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("check update result: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("source not found: %s", id)
	}
	return nil
}

// UpdateLastFetched updates a source's last_fetched_at timestamp.
func (s *Store) UpdateLastFetched(id string, fetchedAt int64) error {
	result, err := s.db.Exec(`
		UPDATE sources SET last_fetched_at = ?
		WHERE id = ?`,
		fetchedAt, id,
	)
	if err != nil {
		return fmt.Errorf("update last fetched %q: %w", id, err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("check update result: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("source not found: %s", id)
	}
	return nil
}

// CountPages returns the total number of pages in the store.
func (s *Store) CountPages() (int, error) {
	var count int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM pages`).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count pages: %w", err)
	}
	return count, nil
}

// CountPagesBySource returns the number of pages for a given source ID.
func (s *Store) CountPagesBySource(sourceID string) (int, error) {
	var count int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM pages WHERE source_id = ?`, sourceID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count pages for source %q: %w", sourceID, err)
	}
	return count, nil
}

// IsDiskSpaceError checks if an error is related to disk space exhaustion.
// When disk space is insufficient, Dewey should continue operating from
// the in-memory index without crashing (edge case from spec).
func IsDiskSpaceError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	// SQLite reports "database or disk is full" for disk space issues.
	return contains(msg, "disk is full") ||
		contains(msg, "no space left") ||
		contains(msg, "SQLITE_FULL")
}

// contains is a case-insensitive substring check.
func contains(s, substr string) bool {
	return len(s) >= len(substr) &&
		(s == substr || len(substr) == 0 ||
			findSubstring(s, substr))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		match := true
		for j := 0; j < len(substr); j++ {
			sc := s[i+j]
			tc := substr[j]
			// Simple ASCII lowercase comparison.
			if sc >= 'A' && sc <= 'Z' {
				sc += 32
			}
			if tc >= 'A' && tc <= 'Z' {
				tc += 32
			}
			if sc != tc {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

// boolToInt converts a bool to an integer for SQLite storage.
func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
