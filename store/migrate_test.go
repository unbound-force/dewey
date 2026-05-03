package store

import (
	"database/sql"
	"fmt"
	"strings"
	"testing"
)

// --- Schema Migration v1→v2 Tests (T008) ---

// TestMigrateV1toV2_ColumnsAdded verifies that the v1→v2 migration adds
// tier and category columns to the pages table.
func TestMigrateV1toV2_ColumnsAdded(t *testing.T) {
	s := newTestStore(t)

	// Verify tier column exists by inserting a page and reading it back.
	p := testPage("migration-test")
	if err := s.InsertPage(p); err != nil {
		t.Fatalf("InsertPage: %v", err)
	}

	got, err := s.GetPage("migration-test")
	if err != nil {
		t.Fatalf("GetPage: %v", err)
	}
	if got == nil {
		t.Fatal("GetPage returned nil")
	}

	// Default tier for non-learning pages should be "authored".
	if got.Tier != "authored" {
		t.Errorf("Tier = %q, want %q", got.Tier, "authored")
	}
	// Category should be empty for non-learning pages.
	if got.Category != "" {
		t.Errorf("Category = %q, want empty", got.Category)
	}
}

// TestMigrateV1toV2_LearningPagesBackfilled verifies that existing learning
// pages are backfilled with tier='draft' during migration.
func TestMigrateV1toV2_LearningPagesBackfilled(t *testing.T) {
	// Create a v1 schema manually, insert a learning page, then run migration.
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer func() { _ = db.Close() }()

	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	// Create v1 schema (without tier/category columns).
	v1Schema := `
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
		CREATE TABLE IF NOT EXISTS blocks (
			uuid TEXT PRIMARY KEY,
			page_name TEXT NOT NULL REFERENCES pages(name) ON DELETE CASCADE,
			parent_uuid TEXT REFERENCES blocks(uuid),
			content TEXT NOT NULL,
			heading_level INTEGER DEFAULT 0,
			position INTEGER DEFAULT 0
		);
		CREATE TABLE IF NOT EXISTS links (
			from_page TEXT NOT NULL REFERENCES pages(name) ON DELETE CASCADE,
			to_page TEXT NOT NULL,
			block_uuid TEXT REFERENCES blocks(uuid) ON DELETE CASCADE,
			PRIMARY KEY (from_page, to_page, block_uuid)
		);
		CREATE TABLE IF NOT EXISTS embeddings (
			block_uuid TEXT NOT NULL REFERENCES blocks(uuid) ON DELETE CASCADE,
			model_id TEXT NOT NULL,
			vector BLOB NOT NULL,
			chunk_text TEXT NOT NULL,
			generated_at INTEGER NOT NULL,
			PRIMARY KEY (block_uuid, model_id)
		);
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
		CREATE TABLE IF NOT EXISTS metadata (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL
		);
	`
	if _, err := db.Exec(v1Schema); err != nil {
		t.Fatalf("create v1 schema: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO metadata (key, value) VALUES ('schema_version', '1')`); err != nil {
		t.Fatalf("set v1 version: %v", err)
	}

	// Insert a learning page (v1 schema — no tier/category columns).
	if _, err := db.Exec(`
		INSERT INTO pages (name, original_name, source_id, source_doc_id, properties, content_hash, is_journal, created_at, updated_at)
		VALUES ('learning/auth-1', 'learning/auth-1', 'learning', 'auth-1', '{}', 'hash1', 0, 1000, 1000)
	`); err != nil {
		t.Fatalf("insert learning page: %v", err)
	}

	// Insert a compiled page.
	if _, err := db.Exec(`
		INSERT INTO pages (name, original_name, source_id, source_doc_id, properties, content_hash, is_journal, created_at, updated_at)
		VALUES ('compiled/auth', 'compiled/auth', 'compiled', 'auth', '{}', 'hash2', 0, 1000, 1000)
	`); err != nil {
		t.Fatalf("insert compiled page: %v", err)
	}

	// Insert a regular disk page.
	if _, err := db.Exec(`
		INSERT INTO pages (name, original_name, source_id, source_doc_id, properties, content_hash, is_journal, created_at, updated_at)
		VALUES ('my-notes', 'my-notes', 'disk-local', 'my-notes.md', '{}', 'hash3', 0, 1000, 1000)
	`); err != nil {
		t.Fatalf("insert disk page: %v", err)
	}

	// Wrap the raw db in a Store and run migration.
	s := &Store{db: db, path: ":memory:"}
	if err := s.migrateV1toV2(); err != nil {
		t.Fatalf("migrateV1toV2: %v", err)
	}

	// Verify learning page has tier='draft'.
	learningPage, err := s.GetPage("learning/auth-1")
	if err != nil {
		t.Fatalf("GetPage(learning): %v", err)
	}
	if learningPage.Tier != "draft" {
		t.Errorf("learning page Tier = %q, want %q", learningPage.Tier, "draft")
	}

	// Verify compiled page has tier='draft'.
	compiledPage, err := s.GetPage("compiled/auth")
	if err != nil {
		t.Fatalf("GetPage(compiled): %v", err)
	}
	if compiledPage.Tier != "draft" {
		t.Errorf("compiled page Tier = %q, want %q", compiledPage.Tier, "draft")
	}

	// Verify regular page has tier='authored' (the default).
	diskPage, err := s.GetPage("my-notes")
	if err != nil {
		t.Fatalf("GetPage(disk): %v", err)
	}
	if diskPage.Tier != "authored" {
		t.Errorf("disk page Tier = %q, want %q", diskPage.Tier, "authored")
	}

	// Verify schema version was updated to 2.
	version, err := s.GetMeta("schema_version")
	if err != nil {
		t.Fatalf("GetMeta(schema_version): %v", err)
	}
	if version != "2" {
		t.Errorf("schema_version = %q, want %q", version, "2")
	}
}

// TestMigrateV1toV2_Idempotent verifies that running the migration twice
// does not produce an error. This is required because modernc.org/sqlite
// does not support ADD COLUMN IF NOT EXISTS.
func TestMigrateV1toV2_Idempotent(t *testing.T) {
	// Create a v1 schema, run migration twice.
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer func() { _ = db.Close() }()

	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	v1Schema := `
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
		CREATE TABLE IF NOT EXISTS blocks (
			uuid TEXT PRIMARY KEY,
			page_name TEXT NOT NULL REFERENCES pages(name) ON DELETE CASCADE,
			parent_uuid TEXT REFERENCES blocks(uuid),
			content TEXT NOT NULL,
			heading_level INTEGER DEFAULT 0,
			position INTEGER DEFAULT 0
		);
		CREATE TABLE IF NOT EXISTS links (
			from_page TEXT NOT NULL REFERENCES pages(name) ON DELETE CASCADE,
			to_page TEXT NOT NULL,
			block_uuid TEXT REFERENCES blocks(uuid) ON DELETE CASCADE,
			PRIMARY KEY (from_page, to_page, block_uuid)
		);
		CREATE TABLE IF NOT EXISTS embeddings (
			block_uuid TEXT NOT NULL REFERENCES blocks(uuid) ON DELETE CASCADE,
			model_id TEXT NOT NULL,
			vector BLOB NOT NULL,
			chunk_text TEXT NOT NULL,
			generated_at INTEGER NOT NULL,
			PRIMARY KEY (block_uuid, model_id)
		);
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
		CREATE TABLE IF NOT EXISTS metadata (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL
		);
	`
	if _, err := db.Exec(v1Schema); err != nil {
		t.Fatalf("create v1 schema: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO metadata (key, value) VALUES ('schema_version', '1')`); err != nil {
		t.Fatalf("set v1 version: %v", err)
	}

	s := &Store{db: db, path: ":memory:"}

	// First migration.
	if err := s.migrateV1toV2(); err != nil {
		t.Fatalf("first migrateV1toV2: %v", err)
	}

	// Second migration — must not error (idempotent).
	if err := s.migrateV1toV2(); err != nil {
		t.Fatalf("second migrateV1toV2 should be idempotent: %v", err)
	}

	// Verify schema is still correct.
	version, err := s.GetMeta("schema_version")
	if err != nil {
		t.Fatalf("GetMeta: %v", err)
	}
	if version != "2" {
		t.Errorf("schema_version = %q, want %q", version, "2")
	}
}

// TestMigrateV1toV2_PreservesExistingData verifies that the migration
// does not modify existing page content, blocks, links, or embeddings.
func TestMigrateV1toV2_PreservesExistingData(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer func() { _ = db.Close() }()

	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	if _, err := db.Exec("PRAGMA foreign_keys=ON"); err != nil {
		t.Fatalf("enable foreign keys: %v", err)
	}

	v1Schema := `
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
		CREATE TABLE IF NOT EXISTS blocks (
			uuid TEXT PRIMARY KEY,
			page_name TEXT NOT NULL REFERENCES pages(name) ON DELETE CASCADE,
			parent_uuid TEXT REFERENCES blocks(uuid),
			content TEXT NOT NULL,
			heading_level INTEGER DEFAULT 0,
			position INTEGER DEFAULT 0
		);
		CREATE TABLE IF NOT EXISTS links (
			from_page TEXT NOT NULL REFERENCES pages(name) ON DELETE CASCADE,
			to_page TEXT NOT NULL,
			block_uuid TEXT REFERENCES blocks(uuid) ON DELETE CASCADE,
			PRIMARY KEY (from_page, to_page, block_uuid)
		);
		CREATE TABLE IF NOT EXISTS embeddings (
			block_uuid TEXT NOT NULL REFERENCES blocks(uuid) ON DELETE CASCADE,
			model_id TEXT NOT NULL,
			vector BLOB NOT NULL,
			chunk_text TEXT NOT NULL,
			generated_at INTEGER NOT NULL,
			PRIMARY KEY (block_uuid, model_id)
		);
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
		CREATE TABLE IF NOT EXISTS metadata (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL
		);
	`
	if _, err := db.Exec(v1Schema); err != nil {
		t.Fatalf("create v1 schema: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO metadata (key, value) VALUES ('schema_version', '1')`); err != nil {
		t.Fatalf("set v1 version: %v", err)
	}

	// Insert a page with a block.
	if _, err := db.Exec(`
		INSERT INTO pages (name, original_name, source_id, source_doc_id, properties, content_hash, is_journal, created_at, updated_at)
		VALUES ('test-page', 'test-page', 'disk-local', 'test.md', '{"key":"value"}', 'hash123', 0, 1000, 2000)
	`); err != nil {
		t.Fatalf("insert page: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO blocks (uuid, page_name, content, heading_level, position)
		VALUES ('block-1', 'test-page', 'block content here', 2, 0)
	`); err != nil {
		t.Fatalf("insert block: %v", err)
	}

	s := &Store{db: db, path: ":memory:"}
	if err := s.migrateV1toV2(); err != nil {
		t.Fatalf("migrateV1toV2: %v", err)
	}

	// Verify page data is preserved.
	page, err := s.GetPage("test-page")
	if err != nil {
		t.Fatalf("GetPage: %v", err)
	}
	if page.OriginalName != "test-page" {
		t.Errorf("OriginalName = %q, want %q", page.OriginalName, "test-page")
	}
	if page.Properties != `{"key":"value"}` {
		t.Errorf("Properties = %q, want preserved value", page.Properties)
	}
	if page.ContentHash != "hash123" {
		t.Errorf("ContentHash = %q, want %q", page.ContentHash, "hash123")
	}
	if page.CreatedAt != 1000 {
		t.Errorf("CreatedAt = %d, want 1000", page.CreatedAt)
	}
	if page.UpdatedAt != 2000 {
		t.Errorf("UpdatedAt = %d, want 2000", page.UpdatedAt)
	}

	// Verify block is preserved.
	block, err := s.GetBlock("block-1")
	if err != nil {
		t.Fatalf("GetBlock: %v", err)
	}
	if block == nil {
		t.Fatal("block should be preserved after migration")
	}
	if block.Content != "block content here" {
		t.Errorf("block Content = %q, want %q", block.Content, "block content here")
	}
}

// --- UpdatePageTier Tests ---

func TestUpdatePageTier_Success(t *testing.T) {
	s := newTestStore(t)

	p := &Page{
		Name:         "learning/auth-1",
		OriginalName: "learning/auth-1",
		SourceID:     "learning",
		SourceDocID:  "auth-1",
		Tier:         "draft",
		CreatedAt:    1000,
		UpdatedAt:    1000,
	}
	if err := s.InsertPage(p); err != nil {
		t.Fatalf("InsertPage: %v", err)
	}

	if err := s.UpdatePageTier("learning/auth-1", "validated"); err != nil {
		t.Fatalf("UpdatePageTier: %v", err)
	}

	got, err := s.GetPage("learning/auth-1")
	if err != nil {
		t.Fatalf("GetPage: %v", err)
	}
	if got.Tier != "validated" {
		t.Errorf("Tier = %q, want %q", got.Tier, "validated")
	}
	// UpdatedAt should be refreshed.
	if got.UpdatedAt <= 1000 {
		t.Error("UpdatedAt should be refreshed after tier update")
	}
}

func TestUpdatePageTier_NotFound(t *testing.T) {
	s := newTestStore(t)

	err := s.UpdatePageTier("nonexistent-page", "validated")
	if err == nil {
		t.Fatal("UpdatePageTier should return error for nonexistent page")
	}
	if !strings.Contains(err.Error(), "page not found") {
		t.Errorf("error = %q, want to contain 'page not found'", err.Error())
	}
}

// --- ListLearningPages Tests ---

func TestListLearningPages_Empty(t *testing.T) {
	s := newTestStore(t)

	pages, err := s.ListLearningPages()
	if err != nil {
		t.Fatalf("ListLearningPages: %v", err)
	}
	if len(pages) != 0 {
		t.Errorf("ListLearningPages returned %d pages, want 0", len(pages))
	}
}

func TestListLearningPages_MixedSources(t *testing.T) {
	s := newTestStore(t)

	// Insert learning pages.
	for i := 1; i <= 3; i++ {
		p := &Page{
			Name:         fmt.Sprintf("learning/auth-%d", i),
			OriginalName: fmt.Sprintf("learning/auth-%d", i),
			SourceID:     "learning",
			SourceDocID:  fmt.Sprintf("auth-%d", i),
			Tier:         "draft",
			CreatedAt:    int64(i * 1000),
			UpdatedAt:    int64(i * 1000),
		}
		if err := s.InsertPage(p); err != nil {
			t.Fatalf("InsertPage(%s): %v", p.Name, err)
		}
	}

	// Insert non-learning pages.
	diskPage := testPage("my-notes")
	if err := s.InsertPage(diskPage); err != nil {
		t.Fatalf("InsertPage(disk): %v", err)
	}

	compiledPage := &Page{
		Name:         "compiled/auth",
		OriginalName: "compiled/auth",
		SourceID:     "compiled",
		SourceDocID:  "auth",
		Tier:         "draft",
		CreatedAt:    1000,
		UpdatedAt:    1000,
	}
	if err := s.InsertPage(compiledPage); err != nil {
		t.Fatalf("InsertPage(compiled): %v", err)
	}

	// ListLearningPages should return only the 3 learning pages.
	pages, err := s.ListLearningPages()
	if err != nil {
		t.Fatalf("ListLearningPages: %v", err)
	}
	if len(pages) != 3 {
		t.Fatalf("ListLearningPages returned %d pages, want 3", len(pages))
	}

	// Verify all returned pages have source_id = "learning".
	for _, p := range pages {
		if p.SourceID != "learning" {
			t.Errorf("page %q has SourceID %q, want %q", p.Name, p.SourceID, "learning")
		}
	}

	// Verify ordering by name.
	if pages[0].Name != "learning/auth-1" {
		t.Errorf("pages[0].Name = %q, want %q", pages[0].Name, "learning/auth-1")
	}
	if pages[2].Name != "learning/auth-3" {
		t.Errorf("pages[2].Name = %q, want %q", pages[2].Name, "learning/auth-3")
	}
}

// --- PagesWithoutEmbeddings Tests ---

func TestPagesWithoutEmbeddings_NoBlocks(t *testing.T) {
	s := newTestStore(t)

	// Page with no blocks should NOT appear (we need blocks but no embeddings).
	if err := s.InsertPage(testPage("no-blocks")); err != nil {
		t.Fatalf("InsertPage: %v", err)
	}

	pages, err := s.PagesWithoutEmbeddings()
	if err != nil {
		t.Fatalf("PagesWithoutEmbeddings: %v", err)
	}
	if len(pages) != 0 {
		t.Errorf("PagesWithoutEmbeddings returned %d pages, want 0 (page has no blocks)", len(pages))
	}
}

func TestPagesWithoutEmbeddings_BlocksNoEmbeddings(t *testing.T) {
	s := newTestStore(t)

	// Page with blocks but no embeddings should appear.
	if err := s.InsertPage(testPage("gap-page")); err != nil {
		t.Fatalf("InsertPage: %v", err)
	}
	if err := s.InsertBlock(testBlock("block-gap-1", "gap-page")); err != nil {
		t.Fatalf("InsertBlock: %v", err)
	}
	if err := s.InsertBlock(testBlock("block-gap-2", "gap-page")); err != nil {
		t.Fatalf("InsertBlock: %v", err)
	}

	pages, err := s.PagesWithoutEmbeddings()
	if err != nil {
		t.Fatalf("PagesWithoutEmbeddings: %v", err)
	}
	if len(pages) != 1 {
		t.Fatalf("PagesWithoutEmbeddings returned %d pages, want 1", len(pages))
	}
	if pages[0].Name != "gap-page" {
		t.Errorf("page name = %q, want %q", pages[0].Name, "gap-page")
	}
}

func TestPagesWithoutEmbeddings_AllEmbedded(t *testing.T) {
	s := newTestStore(t)

	// Page with blocks AND embeddings should NOT appear.
	if err := s.InsertPage(testPage("embedded-page")); err != nil {
		t.Fatalf("InsertPage: %v", err)
	}
	if err := s.InsertBlock(testBlock("block-emb-1", "embedded-page")); err != nil {
		t.Fatalf("InsertBlock: %v", err)
	}
	if err := s.InsertEmbedding("block-emb-1", "model-a", []float32{1, 0, 0}, "chunk"); err != nil {
		t.Fatalf("InsertEmbedding: %v", err)
	}

	pages, err := s.PagesWithoutEmbeddings()
	if err != nil {
		t.Fatalf("PagesWithoutEmbeddings: %v", err)
	}
	if len(pages) != 0 {
		t.Errorf("PagesWithoutEmbeddings returned %d pages, want 0 (all blocks have embeddings)", len(pages))
	}
}

func TestPagesWithoutEmbeddings_PartialEmbeddings(t *testing.T) {
	s := newTestStore(t)

	// Page with some blocks embedded and some not should appear.
	if err := s.InsertPage(testPage("partial-page")); err != nil {
		t.Fatalf("InsertPage: %v", err)
	}
	if err := s.InsertBlock(testBlock("block-partial-1", "partial-page")); err != nil {
		t.Fatalf("InsertBlock: %v", err)
	}
	if err := s.InsertBlock(testBlock("block-partial-2", "partial-page")); err != nil {
		t.Fatalf("InsertBlock: %v", err)
	}
	// Only embed the first block.
	if err := s.InsertEmbedding("block-partial-1", "model-a", []float32{1, 0, 0}, "chunk"); err != nil {
		t.Fatalf("InsertEmbedding: %v", err)
	}

	pages, err := s.PagesWithoutEmbeddings()
	if err != nil {
		t.Fatalf("PagesWithoutEmbeddings: %v", err)
	}
	if len(pages) != 1 {
		t.Fatalf("PagesWithoutEmbeddings returned %d pages, want 1 (partial embeddings)", len(pages))
	}
	if pages[0].Name != "partial-page" {
		t.Errorf("page name = %q, want %q", pages[0].Name, "partial-page")
	}
}

func TestPagesWithoutEmbeddings_NoDuplicates(t *testing.T) {
	s := newTestStore(t)

	// Page with multiple blocks and no embeddings should appear exactly once.
	if err := s.InsertPage(testPage("multi-block-page")); err != nil {
		t.Fatalf("InsertPage: %v", err)
	}
	for i := 0; i < 5; i++ {
		if err := s.InsertBlock(testBlock(fmt.Sprintf("block-multi-%d", i), "multi-block-page")); err != nil {
			t.Fatalf("InsertBlock: %v", err)
		}
	}

	pages, err := s.PagesWithoutEmbeddings()
	if err != nil {
		t.Fatalf("PagesWithoutEmbeddings: %v", err)
	}
	if len(pages) != 1 {
		t.Errorf("PagesWithoutEmbeddings returned %d pages, want 1 (DISTINCT should prevent duplicates)", len(pages))
	}
}

// --- Page Tier and Category in CRUD Tests ---

func TestInsertPage_TierAndCategory(t *testing.T) {
	s := newTestStore(t)

	p := &Page{
		Name:         "learning/test-1",
		OriginalName: "learning/test-1",
		SourceID:     "learning",
		SourceDocID:  "test-1",
		Tier:         "draft",
		Category:     "decision",
		CreatedAt:    1000,
		UpdatedAt:    1000,
	}
	if err := s.InsertPage(p); err != nil {
		t.Fatalf("InsertPage: %v", err)
	}

	got, err := s.GetPage("learning/test-1")
	if err != nil {
		t.Fatalf("GetPage: %v", err)
	}
	if got.Tier != "draft" {
		t.Errorf("Tier = %q, want %q", got.Tier, "draft")
	}
	if got.Category != "decision" {
		t.Errorf("Category = %q, want %q", got.Category, "decision")
	}
}

func TestInsertPage_DefaultTier(t *testing.T) {
	s := newTestStore(t)

	// Page with empty Tier should default to "authored".
	p := testPage("default-tier-page")
	if err := s.InsertPage(p); err != nil {
		t.Fatalf("InsertPage: %v", err)
	}

	got, err := s.GetPage("default-tier-page")
	if err != nil {
		t.Fatalf("GetPage: %v", err)
	}
	if got.Tier != "authored" {
		t.Errorf("Tier = %q, want %q (should default to authored)", got.Tier, "authored")
	}
}

func TestUpdatePage_TierAndCategory(t *testing.T) {
	s := newTestStore(t)

	p := &Page{
		Name:         "learning/update-tier",
		OriginalName: "learning/update-tier",
		SourceID:     "learning",
		SourceDocID:  "update-tier",
		Tier:         "draft",
		Category:     "pattern",
		CreatedAt:    1000,
		UpdatedAt:    1000,
	}
	if err := s.InsertPage(p); err != nil {
		t.Fatalf("InsertPage: %v", err)
	}

	// Update tier and category via UpdatePage.
	p.Tier = "validated"
	p.Category = "decision"
	if err := s.UpdatePage(p); err != nil {
		t.Fatalf("UpdatePage: %v", err)
	}

	got, err := s.GetPage("learning/update-tier")
	if err != nil {
		t.Fatalf("GetPage: %v", err)
	}
	if got.Tier != "validated" {
		t.Errorf("Tier = %q, want %q", got.Tier, "validated")
	}
	if got.Category != "decision" {
		t.Errorf("Category = %q, want %q", got.Category, "decision")
	}
}

func TestListPages_IncludesTierAndCategory(t *testing.T) {
	s := newTestStore(t)

	p := &Page{
		Name:         "learning/list-test",
		OriginalName: "learning/list-test",
		SourceID:     "learning",
		SourceDocID:  "list-test",
		Tier:         "draft",
		Category:     "gotcha",
		CreatedAt:    1000,
		UpdatedAt:    1000,
	}
	if err := s.InsertPage(p); err != nil {
		t.Fatalf("InsertPage: %v", err)
	}

	pages, err := s.ListPages()
	if err != nil {
		t.Fatalf("ListPages: %v", err)
	}
	if len(pages) != 1 {
		t.Fatalf("ListPages returned %d pages, want 1", len(pages))
	}
	if pages[0].Tier != "draft" {
		t.Errorf("Tier = %q, want %q", pages[0].Tier, "draft")
	}
	if pages[0].Category != "gotcha" {
		t.Errorf("Category = %q, want %q", pages[0].Category, "gotcha")
	}
}

func TestListPagesExcludingSource_IncludesTierAndCategory(t *testing.T) {
	s := newTestStore(t)

	p := &Page{
		Name:         "learning/exclude-test",
		OriginalName: "learning/exclude-test",
		SourceID:     "learning",
		SourceDocID:  "exclude-test",
		Tier:         "draft",
		Category:     "context",
		CreatedAt:    1000,
		UpdatedAt:    1000,
	}
	if err := s.InsertPage(p); err != nil {
		t.Fatalf("InsertPage: %v", err)
	}

	pages, err := s.ListPagesExcludingSource("disk-local")
	if err != nil {
		t.Fatalf("ListPagesExcludingSource: %v", err)
	}
	if len(pages) != 1 {
		t.Fatalf("ListPagesExcludingSource returned %d pages, want 1", len(pages))
	}
	if pages[0].Tier != "draft" {
		t.Errorf("Tier = %q, want %q", pages[0].Tier, "draft")
	}
	if pages[0].Category != "context" {
		t.Errorf("Category = %q, want %q", pages[0].Category, "context")
	}
}

func TestListPagesBySource_IncludesTierAndCategory(t *testing.T) {
	s := newTestStore(t)

	p := &Page{
		Name:         "learning/source-test",
		OriginalName: "learning/source-test",
		SourceID:     "learning",
		SourceDocID:  "source-test",
		Tier:         "draft",
		Category:     "reference",
		CreatedAt:    1000,
		UpdatedAt:    1000,
	}
	if err := s.InsertPage(p); err != nil {
		t.Fatalf("InsertPage: %v", err)
	}

	pages, err := s.ListPagesBySource("learning")
	if err != nil {
		t.Fatalf("ListPagesBySource: %v", err)
	}
	if len(pages) != 1 {
		t.Fatalf("ListPagesBySource returned %d pages, want 1", len(pages))
	}
	if pages[0].Tier != "draft" {
		t.Errorf("Tier = %q, want %q", pages[0].Tier, "draft")
	}
	if pages[0].Category != "reference" {
		t.Errorf("Category = %q, want %q", pages[0].Category, "reference")
	}
}
