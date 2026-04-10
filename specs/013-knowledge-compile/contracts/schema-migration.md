# Contract: Schema Migration v1 → v2

**Package**: `store`
**File**: `store/migrate.go`

## Constants

```go
// schemaVersion is the current schema version.
const schemaVersion = 2
```

## Migration: v1 → v2

```go
// migrateV1toV2 adds tier and category columns to the pages table
// for contamination separation (FR-020) and category-aware compilation (FR-008).
// Existing learning pages are backfilled with tier = 'draft'.
// Existing compiled pages (if any) are backfilled with tier = 'draft'.
// All other pages default to tier = 'authored'.
func (s *Store) migrateV1toV2() error
```

### SQL Statements

```sql
-- Add tier column with default 'authored' for human-written content
ALTER TABLE pages ADD COLUMN tier TEXT DEFAULT 'authored';

-- Add category column for learning categorization
ALTER TABLE pages ADD COLUMN category TEXT;

-- Backfill: learning pages are draft tier
UPDATE pages SET tier = 'draft' WHERE source_id = 'learning';

-- Backfill: compiled pages are draft tier
UPDATE pages SET tier = 'draft' WHERE source_id = 'compiled';

-- Index for tier-filtered search performance
CREATE INDEX IF NOT EXISTS idx_pages_tier ON pages(tier);

-- Update schema version
UPDATE metadata SET value = '2' WHERE key = 'schema_version';
```

## Modified Struct: Page

```go
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
    Tier         string // NEW: "authored", "validated", or "draft"
    Category     string // NEW: "decision", "pattern", "gotcha", "context", "reference", or ""
}
```

## Modified Functions

All functions that read/write `Page` structs must be updated to include `tier` and `category` columns:

- `InsertPage(p *Page)` — include `tier` and `category` in INSERT
- `GetPage(name string)` — scan `tier` and `category` from SELECT
- `ListPages()` — scan `tier` and `category`
- `UpdatePage(p *Page)` — include `tier` and `category` in UPDATE
- `ListPagesExcludingSource(sourceID string)` — scan `tier` and `category`
- `ListPagesBySource(sourceID string)` — scan `tier` and `category`

## Modified Function: createSchema()

The `createSchema()` function must include `tier` and `category` columns in the initial CREATE TABLE statement (for fresh databases):

```sql
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
```

## Invariants

1. `migrateV1toV2()` MUST be idempotent — running it twice must not error
2. `tier` MUST default to `'authored'` for all non-learning, non-compiled pages
3. `tier` MUST be `'draft'` for all pages with `source_id = 'learning'`
4. `category` MUST be NULL for non-learning pages
5. The migration MUST NOT modify existing page content, blocks, links, or embeddings
6. The migration MUST complete within a single transaction for atomicity
7. If migration fails, the caller should discard the database and re-index
