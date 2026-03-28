# Data Model: Unified Content Serve

**Branch**: `004-unified-content-serve` | **Date**: 2026-03-28

## Entities

### cachedPage (modified)

The in-memory representation of a page in the vault's index. Extended with source tracking and write protection.

| Field | Type | Description |
|-------|------|-------------|
| entity | types.PageEntity | Page metadata (name, properties, timestamps, journal flag) |
| lowerName | string | Lowercase page name used as map key |
| filePath | string | Relative path for disk pages; source doc ID for external pages |
| blocks | []types.BlockEntity | Hierarchical block tree (content sections) |
| sourceID | string | **NEW**: Origin source identifier (e.g., "disk-local", "github-myorg", "web-docs") |
| readOnly | bool | **NEW**: True for external sources, false for local/MCP-created pages |

**Validation rules**:
- `sourceID` MUST be non-empty for all pages
- `readOnly` MUST be `true` when `sourceID` is not "disk-local"
- `lowerName` MUST be unique across all pages (enforced by the `pages` map key)

### store.Page (unchanged schema)

The SQLite persistent representation. No schema changes needed — the existing columns support all requirements.

| Column | Type | Description |
|--------|------|-------------|
| name | TEXT PK | Page name (namespaced for external: `sourceID/docID`) |
| original_name | TEXT | Display name (may differ from key) |
| source_id | TEXT NOT NULL | Source identifier |
| source_doc_id | TEXT | Source-specific document ID |
| properties | TEXT | JSON-encoded frontmatter properties |
| content_hash | TEXT | SHA-256 of content for change detection |
| is_journal | INTEGER | 0 or 1 |
| created_at | INTEGER | Unix millisecond timestamp |
| updated_at | INTEGER | Unix millisecond timestamp |

**Unique constraint**: `(source_id, source_doc_id)`

### store.Block (unchanged schema)

| Column | Type | Description |
|--------|------|-------------|
| uuid | TEXT PK | Deterministic UUID from content + position |
| page_name | TEXT FK→pages.name CASCADE | Owning page |
| parent_uuid | TEXT FK→blocks.uuid | Null for root blocks |
| content | TEXT NOT NULL | Block content (markdown/text) |
| heading_level | INTEGER | 0 for non-heading, 1-6 for headings |
| position | INTEGER | Order within parent |

### store.Link (unchanged schema)

| Column | Type | Description |
|--------|------|-------------|
| from_page | TEXT FK→pages.name CASCADE | Source page name |
| to_page | TEXT | Target page name (may not exist as a page) |
| block_uuid | TEXT FK→blocks.uuid CASCADE | Block containing the link |

**Composite PK**: `(from_page, to_page, block_uuid)`

### store.EmbeddingRecord (unchanged schema)

| Column | Type | Description |
|--------|------|-------------|
| block_uuid | TEXT FK→blocks.uuid CASCADE | Block this embedding represents |
| model_id | TEXT | Embedding model identifier |
| vector | BLOB | float32 array as binary |
| chunk_text | TEXT NOT NULL | Text that was embedded |
| generated_at | INTEGER | Unix millisecond timestamp |

**Composite PK**: `(block_uuid, model_id)`

## Relationships

```text
source.Document (transient)
    ↓ dewey index
store.Page ──1:N──► store.Block ──1:N──► store.EmbeddingRecord
    │                    │
    │                    └──► store.Link (from block content)
    ↓ dewey serve startup
cachedPage (in-memory, with sourceID + readOnly)
    ↓ applyPageIndex()
vault.Client.pages map + blockIndex + backlinks + searchIndex
```

## State Transitions

### External Page Lifecycle

```text
[Not Indexed] ──dewey index──► [Indexed in graph.db]
                                    │
                          dewey serve startup
                                    │
                                    ▼
                           [Loaded in Memory]
                           (queryable via MCP)
                                    │
                          source removed from
                          sources.yaml + re-index
                                    │
                                    ▼
                              [Auto-Purged]
                         (deleted from graph.db)
```

### Content Update Flow

```text
[Indexed] ──content hash changes──► [Stale]
              ↓                         │
         dewey index                    │
              ↓                         │
    delete old blocks/links/embeds      │
    insert new blocks/links/embeds      │
              ↓                         │
         [Re-Indexed] ◄────────────────┘
```

## New Store Methods Required

| Method | Signature | Purpose |
|--------|-----------|---------|
| ListPagesExcludingSource | `(sourceID string) ([]*Page, error)` | Load external pages for vault startup |
| ListPagesBySource | `(sourceID string) ([]*Page, error)` | Per-source reporting for `dewey status` |
| DeletePagesBySource | `(sourceID string) (int64, error)` | Bulk purge for orphan cleanup (FR-013). Returns rows affected. |

## New Vault Functions Required

| Function | Location | Purpose |
|----------|----------|---------|
| ParseDocument | `vault/parse_export.go` | Exported wrapper: frontmatter + block parsing from string |
| reconstructBlockTree | `vault/vault_store.go` | Flat `[]*store.Block` → nested `[]types.BlockEntity` |
| LoadExternalPages | `vault/vault_store.go` | Load non-local pages from store into vault in-memory index |
