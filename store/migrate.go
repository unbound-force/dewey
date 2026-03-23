package store

import "fmt"

// schemaVersion is the current schema version. Incremented when the schema changes.
const schemaVersion = 1

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
	// Currently only version 1 exists, so no migrations to apply.
	// Future migrations would be applied here sequentially.
	return nil
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
			UNIQUE(source_id, source_doc_id)
		);

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
