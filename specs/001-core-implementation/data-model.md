# Data Model: Dewey Core Implementation

**Branch**: `001-core-implementation` | **Date**: 2026-03-22

## Entity Overview

```
Source ──produces──▷ Document ──parsed-into──▷ Page ──contains──▷ Block
                                                                    │
                                                              embedded-as
                                                                    │
                                                                    ▽
                                                               Embedding
```

## Entities

### Page

A document in the knowledge graph. Corresponds to a single Markdown file (disk source) or a normalized document from an external source.

| Field | Description | Constraints |
|-------|-------------|-------------|
| name | Canonical page name (relative path without `.md`) | Unique within a source. Case-insensitive lookup. |
| original_name | Display name preserving original casing | Immutable after creation. |
| source_id | Identifier of the source that produced this page | Required. FK to Source. |
| source_doc_id | Original document identifier from the source | Unique within source. Used for incremental updates. |
| properties | YAML frontmatter key-value pairs | Map of string to any. Null if no frontmatter. |
| content_hash | Hash of the raw content for change detection | Updated on re-index. Used by incremental update logic. |
| is_journal | Whether this is a daily journal page | Derived from daily folder path. |
| created_at | Timestamp when first indexed | Epoch milliseconds. |
| updated_at | Timestamp when last re-indexed | Epoch milliseconds. Updated on content change. |

**State transitions**: Created (first index) → Updated (content changed) → Deleted (file removed or source removed)

### Block

A section within a page, delimited by headings (H1-H6). Forms a hierarchical tree.

| Field | Description | Constraints |
|-------|-------------|-------------|
| uuid | Unique block identifier | Primary key. Persisted via `<!-- id: UUID -->` HTML comments or deterministic hash. |
| page_name | Parent page name | Required. FK to Page. |
| parent_uuid | UUID of the parent block (null for top-level) | Null for root blocks. |
| content | Raw text content of this block (heading + body) | Non-empty. |
| heading_level | Heading depth (1-6, 0 for non-heading content) | Determines nesting in block tree. |
| position | Order within siblings | Integer, 0-based. |

**Relationships**: Block belongs-to Page (many-to-one). Block has-parent Block (self-referencing, nullable). Block has-children Blocks (one-to-many).

### Link

A directed connection between two pages, discovered from `[[wikilinks]]` in content.

| Field | Description | Constraints |
|-------|-------------|-------------|
| from_page | Source page name | FK to Page. |
| to_page | Target page name (may not exist as a page) | Case-insensitive matching. |
| block_uuid | Block containing the link | FK to Block. Provides context for where the link appears. |

**Relationships**: Link connects two Pages. Link belongs-to Block (where the wikilink text appears).

### Embedding

A vector representation of a content chunk, used for semantic similarity search.

| Field | Description | Constraints |
|-------|-------------|-------------|
| block_uuid | Block this embedding represents | FK to Block. Unique (one embedding per block per model). |
| model_id | Identifier of the embedding model used | E.g., `granite-embedding:30m`. |
| vector | Dense vector (float32 array) | Dimension determined by model (384 for granite-embedding:30m). Stored as BLOB. |
| chunk_text | The text that was embedded | Includes heading hierarchy context prefix. |
| generated_at | Timestamp when embedding was generated | Epoch milliseconds. Used to determine if re-embedding is needed. |

**Validation**: If `model_id` changes in configuration, all embeddings with the old model_id MUST be regenerated. The `generated_at` timestamp and `content_hash` of the parent block determine whether re-embedding is needed.

### Source

A configured origin for content. Defined in `.dewey/sources.yaml`.

| Field | Description | Constraints |
|-------|-------------|-------------|
| id | Unique source identifier | Generated from type + name. E.g., `disk-local`, `github-gaze`, `web-go-stdlib`. |
| type | Source type | One of: `disk`, `github`, `web`. Extensible. |
| name | Human-readable source name | Required. Unique within type. |
| config | Source-specific configuration parameters | Map of string to any. Schema depends on type. |
| refresh_interval | How often to re-fetch content | Duration string (e.g., `daily`, `weekly`, `1h`). Null for disk (uses file watcher). |
| last_fetched_at | Timestamp of last successful fetch | Epoch milliseconds. Null if never fetched. |
| status | Current source status | One of: `active`, `error`, `disabled`. |
| error_message | Last error message if status is `error` | Null when status is `active`. |

**Source-specific config schemas**:

- **disk**: `{ path: "." }` (vault root path, default: current directory)
- **github**: `{ org: "unbound-force", repos: ["gaze", "website"], content: ["issues", "pulls", "readme"] }`
- **web**: `{ urls: ["https://pkg.go.dev/std"], depth: 2, rate_limit: "1s" }`

### Document (Transient)

The Document entity from the spec is a transient in-memory type used during source fetching. Documents are normalized to Pages during indexing and are not persisted as a separate entity. The Source interface's `List()` and `Fetch()` methods return Documents, which the indexing pipeline converts to Pages, Blocks, and Links in the store.

### IndexMetadata

Global metadata about the Dewey index state.

| Field | Description | Constraints |
|-------|-------------|-------------|
| key | Metadata key | Primary key. |
| value | Metadata value | String. |

**Standard keys**: `schema_version`, `last_full_index_at`, `embedding_model`, `embedding_dimension`, `page_count`, `block_count`, `embedding_count`.

## SQLite Schema (`.dewey/graph.db`)

```sql
-- Pages table
CREATE TABLE pages (
    name TEXT PRIMARY KEY,
    original_name TEXT NOT NULL,
    source_id TEXT NOT NULL,
    source_doc_id TEXT,
    properties TEXT,          -- JSON
    content_hash TEXT,
    is_journal INTEGER DEFAULT 0,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    UNIQUE(source_id, source_doc_id)
);

-- Blocks table
CREATE TABLE blocks (
    uuid TEXT PRIMARY KEY,
    page_name TEXT NOT NULL REFERENCES pages(name) ON DELETE CASCADE,
    parent_uuid TEXT REFERENCES blocks(uuid),
    content TEXT NOT NULL,
    heading_level INTEGER DEFAULT 0,
    position INTEGER DEFAULT 0
);
CREATE INDEX idx_blocks_page ON blocks(page_name);
CREATE INDEX idx_blocks_parent ON blocks(parent_uuid);

-- Links table
-- Note: to_page intentionally has no FK -- dangling links to non-existent pages are valid in a wiki
CREATE TABLE links (
    from_page TEXT NOT NULL REFERENCES pages(name) ON DELETE CASCADE,
    to_page TEXT NOT NULL,
    block_uuid TEXT REFERENCES blocks(uuid) ON DELETE CASCADE,
    PRIMARY KEY (from_page, to_page, block_uuid)
);
CREATE INDEX idx_links_to ON links(to_page);

-- Embeddings table
CREATE TABLE embeddings (
    block_uuid TEXT NOT NULL REFERENCES blocks(uuid) ON DELETE CASCADE,
    model_id TEXT NOT NULL,
    vector BLOB NOT NULL,         -- float32 array serialized as bytes
    chunk_text TEXT NOT NULL,
    generated_at INTEGER NOT NULL,
    PRIMARY KEY (block_uuid, model_id)
);

-- Sources table
CREATE TABLE sources (
    id TEXT PRIMARY KEY,
    type TEXT NOT NULL,
    name TEXT NOT NULL,
    config TEXT,                   -- JSON
    refresh_interval TEXT,
    last_fetched_at INTEGER,
    status TEXT DEFAULT 'active',
    error_message TEXT,
    UNIQUE(type, name)
);

-- Index metadata
CREATE TABLE metadata (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL
);
```

## Lifecycle & State Transitions

### Index Lifecycle

```
No Index → dewey init → Empty Index
Empty Index → dewey serve (first start) → Full Index (disk source)
Empty Index → dewey index → Full Index (all sources)
Full Index → dewey serve (subsequent) → Incremental Update → Full Index
Full Index → file change (watcher) → Incremental Update → Full Index
Full Index → dewey index → Source Refresh → Full Index
Full Index → corruption detected → Full Re-index → Full Index
```

### Source Lifecycle

```
Configured (in sources.yaml) → First Fetch (dewey index) → Active
Active → Refresh Interval Expired → Stale → Fetch → Active
Active → Fetch Failure → Error (serve from cache)
Error → Retry (next dewey index) → Active or Error
```

### Embedding Lifecycle

```
Block Created/Updated → Chunk Prepared → Embed Request → Embedding Stored
Block Deleted → Embedding Deleted (cascade)
Model Changed → All Embeddings Invalidated → Re-embed All
```

## Schema Migration Strategy

**Initial schema version**: `1`

**Migration behavior on startup**:
1. Open `.dewey/graph.db` and read `schema_version` from the `metadata` table.
2. If the database does not exist, create it with the current schema and set `schema_version` to the latest version.
3. If `schema_version` matches the current code's expected version, proceed normally.
4. If `schema_version` is older than expected, run forward migrations sequentially (version N → N+1 → N+2 → ... → current). Each migration is a SQL script that alters the schema.
5. If a migration fails, log the error, discard the database, and perform a full re-index from scratch with a warning. This is the fallback for unrecoverable schema states.
6. Rollback (downgrade) is not supported. If a user downgrades Dewey, the database is discarded and re-indexed.

**Migration file convention**: Each migration is a numbered SQL file or embedded Go constant (e.g., `migration_002_add_embeddings_table`). Migrations are forward-only and idempotent where possible.
