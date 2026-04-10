package store

import (
	"fmt"
	"strings"
)

// schemaVersion is the current schema version. Incremented when the schema changes.
const schemaVersion = 2

// migrate applies database schema migrations. On first run, it creates all
// tables. On subsequent runs, it checks the schema_version metadata and
// applies forward migrations as needed.
//
// Migration strategy (per data-model.md):
//  1. Read schema_version from metadata table
//  2. If database is new, create all tables and set version
//  3. If version matches, proceed normally
//  4. If version is older, run forward migrations sequentially
//  5. If migration fails, return error (caller should discard and re-index)
func (s *Store) migrate() error {
	// Check if the metadata table exists (indicates an existing database).
	var tableCount int
	err := s.db.QueryRow(`
		SELECT COUNT(*) FROM sqlite_master
		WHERE type='table' AND name='metadata'`).Scan(&tableCount)
	if err != nil {
		return fmt.Errorf("check metadata table: %w", err)
	}

	if tableCount == 0 {
		// Fresh database — create all tables.
		return s.createSchema()
	}

	// Existing database — check version and apply migrations.
	var versionStr string
	err = s.db.QueryRow(`SELECT value FROM metadata WHERE key = 'schema_version'`).Scan(&versionStr)
	if err != nil {
		// No schema_version key — treat as fresh.
		return s.createSchema()
	}

	var currentVersion int
	if _, err := fmt.Sscanf(versionStr, "%d", &currentVersion); err != nil {
		return fmt.Errorf("parse schema version %q: %w", versionStr, err)
	}

	if currentVersion == schemaVersion {
		return nil // Up to date.
	}

	if currentVersion > schemaVersion {
		return fmt.Errorf("database schema version %d is newer than supported version %d", currentVersion, schemaVersion)
	}

	// Apply forward migrations from currentVersion to schemaVersion.
	// Each migration runs in a transaction for atomicity.
	migrations := map[int]func() error{
		1: s.migrateV1toV2,
	}

	for v := currentVersion; v < schemaVersion; v++ {
		migrateFn, ok := migrations[v]
		if !ok {
			return fmt.Errorf("no migration path from version %d to %d", v, v+1)
		}
		if err := migrateFn(); err != nil {
			return fmt.Errorf("migrate v%d to v%d: %w", v, v+1, err)
		}
	}

	return nil
}

// migrateV1toV2 adds tier and category columns to the pages table
// for contamination separation (FR-020) and category-aware compilation (FR-008).
// Existing learning pages are backfilled with tier = 'draft'.
// Existing compiled pages (if any) are backfilled with tier = 'draft'.
// All other pages default to tier = 'authored'.
//
// The migration is idempotent: re-running it on a v2 schema is safe because
// ALTER TABLE errors for duplicate columns are silently ignored.
func (s *Store) migrateV1toV2() error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// Add tier column. modernc.org/sqlite does not support ADD COLUMN IF NOT EXISTS,
	// so we attempt the ALTER and ignore "duplicate column" errors for idempotency.
	if _, err := tx.Exec(`ALTER TABLE pages ADD COLUMN tier TEXT DEFAULT 'authored'`); err != nil {
		if !isDuplicateColumnError(err) {
			return fmt.Errorf("add tier column: %w", err)
		}
	}

	// Add category column.
	if _, err := tx.Exec(`ALTER TABLE pages ADD COLUMN category TEXT`); err != nil {
		if !isDuplicateColumnError(err) {
			return fmt.Errorf("add category column: %w", err)
		}
	}

	// Backfill: learning pages are draft tier.
	if _, err := tx.Exec(`UPDATE pages SET tier = 'draft' WHERE source_id = 'learning'`); err != nil {
		return fmt.Errorf("backfill learning tier: %w", err)
	}

	// Backfill: compiled pages are draft tier.
	if _, err := tx.Exec(`UPDATE pages SET tier = 'draft' WHERE source_id = 'compiled'`); err != nil {
		return fmt.Errorf("backfill compiled tier: %w", err)
	}

	// Create index for tier-filtered search performance.
	if _, err := tx.Exec(`CREATE INDEX IF NOT EXISTS idx_pages_tier ON pages(tier)`); err != nil {
		return fmt.Errorf("create tier index: %w", err)
	}

	// Update schema version.
	if _, err := tx.Exec(`UPDATE metadata SET value = '2' WHERE key = 'schema_version'`); err != nil {
		return fmt.Errorf("update schema version: %w", err)
	}

	return tx.Commit()
}

// isDuplicateColumnError returns true if the error indicates an attempt to
// add a column that already exists. modernc.org/sqlite does not support
// ADD COLUMN IF NOT EXISTS, so we detect this error for idempotency.
func isDuplicateColumnError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "duplicate column") ||
		strings.Contains(msg, "already exists")
}

// createSchema creates all tables for a fresh database.
// Uses the exact DDL from data-model.md.
func (s *Store) createSchema() error {
	schema := `
		-- Pages table
		CREATE TABLE IF NOT EXISTS pages (
			name TEXT PRIMARY KEY,
			original_name TEXT NOT NULL,
			source_id TEXT NOT NULL,
			source_doc_id TEXT,
			properties TEXT,
			content_hash TEXT,
			is_journal INTEGER DEFAULT 0,
			created_at INTEGER NOT NULL,
			updated_at INTEGER NOT NULL,
			tier TEXT DEFAULT 'authored',
			category TEXT,
			UNIQUE(source_id, source_doc_id)
		);
		CREATE INDEX IF NOT EXISTS idx_pages_tier ON pages(tier);

		-- Blocks table
		CREATE TABLE IF NOT EXISTS blocks (
			uuid TEXT PRIMARY KEY,
			page_name TEXT NOT NULL REFERENCES pages(name) ON DELETE CASCADE,
			parent_uuid TEXT REFERENCES blocks(uuid),
			content TEXT NOT NULL,
			heading_level INTEGER DEFAULT 0,
			position INTEGER DEFAULT 0
		);
		CREATE INDEX IF NOT EXISTS idx_blocks_page ON blocks(page_name);
		CREATE INDEX IF NOT EXISTS idx_blocks_parent ON blocks(parent_uuid);

		-- Links table
		CREATE TABLE IF NOT EXISTS links (
			from_page TEXT NOT NULL REFERENCES pages(name) ON DELETE CASCADE,
			to_page TEXT NOT NULL,
			block_uuid TEXT REFERENCES blocks(uuid) ON DELETE CASCADE,
			PRIMARY KEY (from_page, to_page, block_uuid)
		);
		CREATE INDEX IF NOT EXISTS idx_links_to ON links(to_page);

		-- Embeddings table
		CREATE TABLE IF NOT EXISTS embeddings (
			block_uuid TEXT NOT NULL REFERENCES blocks(uuid) ON DELETE CASCADE,
			model_id TEXT NOT NULL,
			vector BLOB NOT NULL,
			chunk_text TEXT NOT NULL,
			generated_at INTEGER NOT NULL,
			PRIMARY KEY (block_uuid, model_id)
		);

		-- Sources table
		CREATE TABLE IF NOT EXISTS sources (
			id TEXT PRIMARY KEY,
			type TEXT NOT NULL,
			name TEXT NOT NULL,
			config TEXT,
			refresh_interval TEXT,
			last_fetched_at INTEGER,
			status TEXT DEFAULT 'active',
			error_message TEXT,
			UNIQUE(type, name)
		);

		-- Index metadata
		CREATE TABLE IF NOT EXISTS metadata (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL
		);
	`

	if _, err := s.db.Exec(schema); err != nil {
		return fmt.Errorf("create schema: %w", err)
	}

	// Set initial schema version.
	if _, err := s.db.Exec(
		`INSERT OR REPLACE INTO metadata (key, value) VALUES ('schema_version', ?)`,
		fmt.Sprintf("%d", schemaVersion),
	); err != nil {
		return fmt.Errorf("set schema version: %w", err)
	}

	return nil
}
