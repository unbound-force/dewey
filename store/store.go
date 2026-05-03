// Package store provides SQLite-backed persistence for the Dewey knowledge graph.
// It stores pages, blocks, links, embeddings, and index metadata in a single
// .uf/dewey/graph.db file using modernc.org/sqlite (pure Go, no CGO).
package store

import (
	"database/sql"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/charmbracelet/log"
	_ "modernc.org/sqlite" // Pure-Go SQLite driver registration.
)

// logger is the package-level structured logger for store operations.
var logger = log.NewWithOptions(os.Stderr, log.Options{
	Prefix:          "dewey/store",
	ReportTimestamp: true,
	TimeFormat:      "2006-01-02T15:04:05.000Z07:00",
})

// SetLogLevel sets the logging level for the store package.
// Use log.DebugLevel for verbose output during diagnostics.
func SetLogLevel(level log.Level) {
	logger.SetLevel(level)
}

// SetLogOutput replaces the store package logger with one that writes to
// the given writer at the given level. Used to enable file logging.
func SetLogOutput(w io.Writer, level log.Level) {
	newLogger := log.NewWithOptions(w, log.Options{
		Prefix:          "dewey/store",
		Level:           level,
		ReportTimestamp: true,
		TimeFormat:      "2006-01-02T15:04:05.000Z07:00",
	})
	*logger = *newLogger
}

// Store wraps a SQLite database connection for knowledge graph persistence.
// It manages pages, blocks, links, embeddings, and index metadata.
// File-level locking prevents concurrent write corruption (T059).
type Store struct {
	db       *sql.DB
	path     string
	lockFile *os.File // File lock for .uf/dewey/ directory (nil for :memory:).
}

// New opens (or creates) a SQLite database at the given path and applies
// schema migrations. Pass an empty string or ":memory:" for an in-memory
// database (useful for testing).
//
// Returns an error if the database cannot be opened, pragma configuration
// fails, the file lock cannot be acquired (another Dewey process is using
// the database), or schema migration fails.
//
// The returned Store must be closed with [Store.Close] when no longer needed
// to release the database connection and file lock.
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
		lockPath := filepath.Join(filepath.Dir(path), "dewey.lock")
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
		// Write PID and command for diagnostic identification (best-effort).
		_, _ = fmt.Fprintf(lockFile, "%d %s\n", os.Getpid(), strings.Join(os.Args, " "))
		logger.Debug("lock acquired", "pid", os.Getpid(), "path", lockPath)
		s.lockFile = lockFile
	}

	if err := s.migrate(); err != nil {
		_ = s.Close()
		return nil, fmt.Errorf("migrate schema: %w", err)
	}

	return s, nil
}

// Close closes the underlying database connection and releases the file lock.
// Returns an error if the database connection cannot be closed cleanly.
func (s *Store) Close() error {
	if s.lockFile != nil {
		logger.Debug("lock released")
		// Truncate PID data so stale lock files don't contain misleading info.
		_ = s.lockFile.Truncate(0)
		// Release the advisory lock before closing the file descriptor.
		_ = syscall.Flock(int(s.lockFile.Fd()), syscall.LOCK_UN)
		_ = s.lockFile.Close()
	}
	return s.db.Close()
}

// DB returns the underlying *sql.DB for advanced queries.
// Prefer using Store methods for standard operations. The returned
// connection is managed by the Store and must not be closed independently.
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
	Tier         string // "authored", "validated", or "draft"
	Category     string // "decision", "pattern", "gotcha", "context", "reference", or ""
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

// InsertPage inserts a new page into the store. It sets CreatedAt and
// UpdatedAt to the current time if they are zero. Uses parameterized
// queries to prevent SQL injection (FR-028).
//
// Returns an error if the insert fails (e.g., duplicate page name
// violating the unique constraint).
func (s *Store) InsertPage(p *Page) error {
	now := time.Now().UnixMilli()
	if p.CreatedAt == 0 {
		p.CreatedAt = now
	}
	if p.UpdatedAt == 0 {
		p.UpdatedAt = now
	}

	// Default tier to "authored" if not explicitly set, matching the schema default.
	tier := p.Tier
	if tier == "" {
		tier = "authored"
	}

	_, err := s.db.Exec(`
		INSERT INTO pages (name, original_name, source_id, source_doc_id, properties, content_hash, is_journal, created_at, updated_at, tier, category)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		p.Name, p.OriginalName, p.SourceID, p.SourceDocID,
		p.Properties, p.ContentHash, boolToInt(p.IsJournal),
		p.CreatedAt, p.UpdatedAt, tier, p.Category,
	)
	if err != nil {
		return fmt.Errorf("insert page %q: %w", p.Name, err)
	}
	return nil
}

// GetPage retrieves a page by its normalized name.
// Returns the page and nil error on success, or (nil, nil) if no page
// exists with the given name. Returns a non-nil error if the query fails.
func (s *Store) GetPage(name string) (*Page, error) {
	p := &Page{}
	var isJournal int
	var sourceDocID, properties, contentHash, tier, category sql.NullString

	err := s.db.QueryRow(`
		SELECT name, original_name, source_id, source_doc_id, properties, content_hash, is_journal, created_at, updated_at, tier, category
		FROM pages WHERE name = ?`, name).Scan(
		&p.Name, &p.OriginalName, &p.SourceID, &sourceDocID,
		&properties, &contentHash, &isJournal,
		&p.CreatedAt, &p.UpdatedAt, &tier, &category,
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
	p.Tier = tier.String
	p.Category = category.String
	return p, nil
}

// ListPages returns all pages in the store, ordered alphabetically by name.
// Returns an empty slice (not nil) if no pages exist. Returns an error if
// the query or row scanning fails.
func (s *Store) ListPages() ([]*Page, error) {
	rows, err := s.db.Query(`
		SELECT name, original_name, source_id, source_doc_id, properties, content_hash, is_journal, created_at, updated_at, tier, category
		FROM pages ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("list pages: %w", err)
	}
	defer func() { _ = rows.Close() }()

	return scanPages(rows)
}

// UpdatePage updates an existing page's mutable fields and sets UpdatedAt
// to the current time. The content_hash comparison enables incremental
// indexing — only re-index when content changes.
//
// Returns an error if the update query fails or if no page exists with
// the given name (page not found).
func (s *Store) UpdatePage(p *Page) error {
	p.UpdatedAt = time.Now().UnixMilli()

	result, err := s.db.Exec(`
		UPDATE pages SET original_name = ?, source_id = ?, source_doc_id = ?,
		properties = ?, content_hash = ?, is_journal = ?, updated_at = ?,
		tier = ?, category = ?
		WHERE name = ?`,
		p.OriginalName, p.SourceID, p.SourceDocID,
		p.Properties, p.ContentHash, boolToInt(p.IsJournal),
		p.UpdatedAt, p.Tier, p.Category, p.Name,
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
// Returns an error if the delete query fails or if no page exists with the
// given name (page not found).
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

// InsertBlock inserts a new block into the store. The block's PageName
// must reference an existing page (foreign key constraint).
//
// Returns an error if the insert fails (e.g., duplicate UUID or missing
// parent page).
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

// GetBlock retrieves a block by its UUID.
// Returns the block and nil error on success, or (nil, nil) if no block
// exists with the given UUID. Returns a non-nil error if the query fails.
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

// GetBlocksByPage returns all blocks belonging to the named page, ordered
// by position. Returns an empty slice if the page has no blocks or does
// not exist. Returns an error if the query or row scanning fails.
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

// DeleteBlocksByPage removes all blocks belonging to the named page.
// Returns an error if the delete query fails. Does not return an error
// if no blocks exist for the page (idempotent delete).
func (s *Store) DeleteBlocksByPage(pageName string) error {
	_, err := s.db.Exec(`DELETE FROM blocks WHERE page_name = ?`, pageName)
	if err != nil {
		return fmt.Errorf("delete blocks for page %q: %w", pageName, err)
	}
	return nil
}

// InsertLink inserts a directed link between two pages. Uses INSERT OR
// IGNORE to silently skip duplicate links (same from_page, to_page,
// block_uuid triple). Returns an error if the insert query fails for
// reasons other than a duplicate.
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

// GetForwardLinks returns all outgoing links from the named page (pages
// that this page links to). Returns an empty slice if the page has no
// outgoing links. Returns an error if the query or row scanning fails.
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

// GetBackwardLinks returns all incoming links to the named page (pages
// that link to this page). Returns an empty slice if no pages link to
// the given page. Returns an error if the query or row scanning fails.
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

// DeleteLinksByPage removes all outgoing links from the named page.
// Returns an error if the delete query fails. Does not return an error
// if no links exist for the page (idempotent delete).
func (s *Store) DeleteLinksByPage(pageName string) error {
	_, err := s.db.Exec(`DELETE FROM links WHERE from_page = ?`, pageName)
	if err != nil {
		return fmt.Errorf("delete links for page %q: %w", pageName, err)
	}
	return nil
}

// GetMeta retrieves a metadata value by key. Returns an empty string and
// nil error if the key does not exist. Returns a non-nil error if the
// query fails.
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

// SetMeta sets a metadata key-value pair, inserting a new entry or updating
// the value if the key already exists (upsert). Returns an error if the
// upsert query fails.
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

// InsertSource inserts a new content source record into the store. Uses
// parameterized queries to prevent SQL injection (FR-028).
//
// Returns an error if the insert fails (e.g., duplicate source ID).
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

// GetSource retrieves a content source record by its ID.
// Returns the record and nil error on success, or (nil, nil) if no source
// exists with the given ID. Returns a non-nil error if the query fails.
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

// ListSources returns all content source records in the store, ordered by ID.
// Returns an empty slice if no sources exist. Returns an error if the query
// or row scanning fails.
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

// UpdateSourceStatus updates a source's status and error message fields.
// Returns an error if the update query fails or if no source exists with
// the given ID (source not found).
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

// UpdateLastFetched updates a source's last_fetched_at timestamp to the
// given Unix millisecond value. Returns an error if the update query fails
// or if no source exists with the given ID (source not found).
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
// Returns 0 and an error if the count query fails.
func (s *Store) CountPages() (int, error) {
	var count int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM pages`).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count pages: %w", err)
	}
	return count, nil
}

// CountPagesBySource returns the number of pages associated with the given
// source ID. Returns 0 (not an error) if no pages belong to that source.
// Returns an error if the count query fails.
func (s *Store) CountPagesBySource(sourceID string) (int, error) {
	var count int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM pages WHERE source_id = ?`, sourceID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count pages for source %q: %w", sourceID, err)
	}
	return count, nil
}

// LatestUpdatedAtBySource returns the most recent updated_at timestamp
// (Unix milliseconds) for pages belonging to the given source ID.
// Returns 0 if no pages belong to the source. Used by lint to detect
// stale knowledge stores (015-curated-knowledge-stores, FR-026).
func (s *Store) LatestUpdatedAtBySource(sourceID string) (int64, error) {
	var latest sql.NullInt64
	err := s.db.QueryRow(`SELECT MAX(updated_at) FROM pages WHERE source_id = ?`, sourceID).Scan(&latest)
	if err != nil {
		return 0, fmt.Errorf("latest updated_at for source %q: %w", sourceID, err)
	}
	if !latest.Valid {
		return 0, nil
	}
	return latest.Int64, nil
}

// ListPagesExcludingSource returns all pages whose source_id does NOT match
// the given sourceID, ordered alphabetically by name. Used by LoadExternalPages()
// to load all non-local pages from the store into the vault's in-memory index.
// Returns an empty slice if no matching pages exist.
func (s *Store) ListPagesExcludingSource(sourceID string) ([]*Page, error) {
	rows, err := s.db.Query(`
		SELECT name, original_name, source_id, source_doc_id, properties, content_hash, is_journal, created_at, updated_at, tier, category
		FROM pages WHERE source_id != ? ORDER BY name`, sourceID)
	if err != nil {
		return nil, fmt.Errorf("list pages excluding source %q: %w", sourceID, err)
	}
	defer func() { _ = rows.Close() }()

	return scanPages(rows)
}

// DeletePagesBySource removes all pages (and their associated blocks, links,
// and embeddings via CASCADE) belonging to the given source ID. Returns the
// number of rows deleted. Used for orphan auto-purge when a source is removed
// from sources.yaml (FR-013). The operation is wrapped in a transaction for
// atomicity.
func (s *Store) DeletePagesBySource(sourceID string) (int64, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return 0, fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	result, err := tx.Exec(`DELETE FROM pages WHERE source_id = ?`, sourceID)
	if err != nil {
		return 0, fmt.Errorf("delete pages for source %q: %w", sourceID, err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("check delete result: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit transaction: %w", err)
	}
	return rows, nil
}

// ListPagesBySource returns all pages belonging to the given source ID,
// ordered alphabetically by name. Used by `dewey status` for per-source
// page count reporting (FR-010). Returns an empty slice if no pages belong
// to the source.
func (s *Store) ListPagesBySource(sourceID string) ([]*Page, error) {
	rows, err := s.db.Query(`
		SELECT name, original_name, source_id, source_doc_id, properties, content_hash, is_journal, created_at, updated_at, tier, category
		FROM pages WHERE source_id = ? ORDER BY name`, sourceID)
	if err != nil {
		return nil, fmt.Errorf("list pages for source %q: %w", sourceID, err)
	}
	defer func() { _ = rows.Close() }()

	return scanPages(rows)
}

// ListPagesBySourceUpdatedAfter returns pages belonging to the given source ID
// that have been updated after the specified Unix millisecond timestamp.
// Ordered by updated_at ascending. Used by incremental curation to process
// only new/changed documents (FR-019, 015-curated-knowledge-stores).
func (s *Store) ListPagesBySourceUpdatedAfter(sourceID string, after int64) ([]*Page, error) {
	rows, err := s.db.Query(`
		SELECT name, original_name, source_id, source_doc_id, properties, content_hash, is_journal, created_at, updated_at, tier, category
		FROM pages WHERE source_id = ? AND updated_at > ? ORDER BY updated_at`, sourceID, after)
	if err != nil {
		return nil, fmt.Errorf("list pages for source %q updated after %d: %w", sourceID, after, err)
	}
	defer func() { _ = rows.Close() }()

	return scanPages(rows)
}

// scanPages scans rows from a page query into a slice of Page pointers.
// Consolidates the repeated scan logic across ListPages, ListPagesExcludingSource,
// ListPagesBySource, and ListLearningPages (DRY — extracted after 3+ duplications).
func scanPages(rows *sql.Rows) ([]*Page, error) {
	var pages []*Page
	for rows.Next() {
		p := &Page{}
		var isJournal int
		var sourceDocID, properties, contentHash, tier, category sql.NullString

		if err := rows.Scan(
			&p.Name, &p.OriginalName, &p.SourceID, &sourceDocID,
			&properties, &contentHash, &isJournal,
			&p.CreatedAt, &p.UpdatedAt, &tier, &category,
		); err != nil {
			return nil, fmt.Errorf("scan page: %w", err)
		}

		p.SourceDocID = sourceDocID.String
		p.Properties = properties.String
		p.ContentHash = contentHash.String
		p.IsJournal = isJournal != 0
		p.Tier = tier.String
		p.Category = category.String
		pages = append(pages, p)
	}
	return pages, rows.Err()
}

// --- Learning and knowledge compilation helpers (013-knowledge-compile) ---

// ListLearningPages returns all pages with source_id = 'learning',
// ordered by name. Used by lint to enumerate all learnings.
func (s *Store) ListLearningPages() ([]*Page, error) {
	rows, err := s.db.Query(`
		SELECT name, original_name, source_id, source_doc_id, properties, content_hash, is_journal, created_at, updated_at, tier, category
		FROM pages WHERE source_id = 'learning' ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("list learning pages: %w", err)
	}
	defer func() { _ = rows.Close() }()

	return scanPages(rows)
}

// PagesWithoutEmbeddings returns pages that have blocks but no
// embeddings. Used by lint to detect embedding gaps (FR-017).
func (s *Store) PagesWithoutEmbeddings() ([]*Page, error) {
	rows, err := s.db.Query(`
		SELECT DISTINCT p.name, p.original_name, p.source_id, p.source_doc_id,
			p.properties, p.content_hash, p.is_journal, p.created_at, p.updated_at,
			p.tier, p.category
		FROM pages p
		JOIN blocks b ON b.page_name = p.name
		LEFT JOIN embeddings e ON e.block_uuid = b.uuid
		WHERE e.block_uuid IS NULL`)
	if err != nil {
		return nil, fmt.Errorf("pages without embeddings: %w", err)
	}
	defer func() { _ = rows.Close() }()

	return scanPages(rows)
}

// UpdatePageTier updates a page's tier column and refreshes the updated_at
// timestamp. Returns an error if the page doesn't exist or the update fails.
// Used by the promote MCP tool to transition pages from draft to validated (FR-023).
func (s *Store) UpdatePageTier(name, tier string) error {
	now := time.Now().UnixMilli()
	result, err := s.db.Exec(`
		UPDATE pages SET tier = ?, updated_at = ? WHERE name = ?`,
		tier, now, name,
	)
	if err != nil {
		return fmt.Errorf("update page tier %q: %w", name, err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("check update result: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("page not found: %s", name)
	}
	return nil
}

// IsDiskSpaceError returns true if the given error indicates disk space
// exhaustion (e.g., SQLite "database or disk is full", OS "no space left").
// Returns false if err is nil. When disk space is insufficient, Dewey
// should continue operating from the in-memory index without crashing.
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
