# Phase 0 Research: Knowledge Compilation & Temporal Intelligence

**Branch**: `013-knowledge-compile` | **Date**: 2026-04-10

## R1: Current `store_learning` Implementation Analysis

The existing `tools/learning.go` (spec 008) stores learnings as pages in the knowledge graph:

1. **Page naming**: `learning/{unix_milli_timestamp}` — e.g., `learning/1712764800000`
2. **Document ID**: `learning-{unix_milli_timestamp}`
3. **Source ID**: `"learning"` (distinguishes from disk/github/web/code sources)
4. **Properties**: JSON with optional `tags` field (comma-separated string)
5. **Content hash**: SHA-256 of the learning text (first 8 bytes, hex)
6. **Blocks**: Parsed via `vault.ParseDocument()`, persisted via `vault.PersistBlocks()`
7. **Embeddings**: Generated via `vault.GenerateEmbeddings()` when Ollama is available

**Key observations**:
- The `tags` field is optional and comma-separated (e.g., `"gotcha, vault-walker, 006-unified-ignore"`)
- No `category` concept exists — all learnings are treated equally
- No `created_at` ISO 8601 field — only the Unix millisecond `CreatedAt` on the `Page` struct (set automatically by `InsertPage`)
- No `tier` concept — all pages have equal trust weight
- The learning identity is a UUID (first block's UUID), not human-readable
- There is currently **1 stored learning** in the production database (per spec assumptions)

**Migration impact**: The single existing learning needs `tag: "general"` backfilled. The `tags` → `tag` API change is breaking but affects only MCP tool callers (agents), not stored data.

## R2: Schema Analysis — Pages Table

Current `pages` table schema (from `store/migrate.go`):

```sql
CREATE TABLE IF NOT EXISTS pages (
    name TEXT PRIMARY KEY,
    original_name TEXT NOT NULL,
    source_id TEXT NOT NULL,
    source_doc_id TEXT,
    properties TEXT,
    content_hash TEXT,
    is_journal INTEGER DEFAULT 0,
    created_at INTEGER NOT NULL,    -- Unix milliseconds
    updated_at INTEGER NOT NULL,    -- Unix milliseconds
    UNIQUE(source_id, source_doc_id)
);
```

**Observations**:
- `created_at` already exists as `INTEGER` (Unix milliseconds). The spec requires ISO 8601 `TEXT` for the new `created_at` field. **Resolution**: Add a new `created_at_iso TEXT` column rather than changing the existing `created_at INTEGER`. The existing column is used throughout the codebase for sorting and comparison. The ISO 8601 column is for MCP tool response metadata only.
- **Revised approach**: Actually, the spec says "Each stored learning MUST include a `created_at` property with ISO 8601 timestamp, set automatically at storage time" (FR-003). This can be achieved by storing the ISO 8601 value in the page's `properties` JSON field, or by adding a dedicated column. Using the properties JSON is simpler and avoids schema migration for this field. However, the `tier` and `category` fields need to be queryable (for filtering), so they should be columns.

**Decision**: 
- `tier TEXT DEFAULT 'authored'` — new column, queryable for search filtering (FR-024)
- `category TEXT` — new column, queryable for lint queries (FR-017)
- `created_at` ISO 8601 — stored in properties JSON (already exists as Unix ms in the column; ISO format derived at query time or stored in properties)

**Revised decision after further analysis**: The existing `created_at INTEGER` column already stores the timestamp. For MCP responses, we convert Unix ms → ISO 8601 at the tool layer (in `toSemanticResults()`). No new column needed for `created_at`. Only `tier` and `category` need new columns.

## R3: Schema Migration Strategy

Current schema version is 1 (from `store/migrate.go` line 6). The migration framework supports forward migrations but has no migration functions yet (the comment says "Currently only version 1 exists, so no migrations to apply").

**Migration v1 → v2**:
```sql
ALTER TABLE pages ADD COLUMN tier TEXT DEFAULT 'authored';
ALTER TABLE pages ADD COLUMN category TEXT;
```

**Data migration**:
- All existing pages get `tier = 'authored'` (the DEFAULT handles this)
- Pages with `source_id = 'learning'` get `tier = 'draft'`
- The single existing learning page gets `category = NULL` (no category assigned)
- The existing learning's properties JSON gets `tag: "general"` added (for the `{tag}-{sequence}` identity system)

**Index considerations**: Add an index on `tier` for filtered search performance:
```sql
CREATE INDEX IF NOT EXISTS idx_pages_tier ON pages(tier);
```

## R4: `{tag}-{sequence}` Identity System

The spec requires learnings to have identities like `authentication-3` (FR-002, FR-005). This is a human-readable, addressable name scoped to the tag namespace.

**Implementation options**:

1. **Query-time sequence**: When storing a learning with `tag: "auth"`, query `SELECT COUNT(*) FROM pages WHERE source_id = 'learning' AND properties LIKE '%"tag":"auth"%'` to determine the next sequence number. Store the identity in the page name: `learning/auth-3`.

2. **Metadata table counter**: Store `tag_sequence:{tag}` in the metadata table. Increment atomically on each store. Simpler but requires metadata table management.

3. **Page name convention**: Change the page name from `learning/{timestamp}` to `learning/{tag}-{sequence}`. The sequence is derived from existing pages with the same tag prefix.

**Selected approach**: Option 3 — page name convention. The page name becomes `learning/{tag}-{sequence}` (e.g., `learning/authentication-3`). The sequence is determined by counting existing pages with `name LIKE 'learning/{tag}-%'` and incrementing. This makes the identity visible in page listings and searchable by name.

**Edge cases**:
- Tag normalization: lowercase, trim whitespace, replace spaces with hyphens
- Concurrent writes: SQLite's single-writer mode prevents race conditions
- Tag with hyphens: `my-tag-1` — the sequence is always the last numeric segment after the tag prefix

**Sequence query**:
```sql
SELECT COUNT(*) FROM pages 
WHERE source_id = 'learning' 
AND name LIKE 'learning/' || ? || '-%'
```

Then the new page name is `learning/{tag}-{count+1}`.

## R5: Compile Tool — LLM Integration Architecture

The compile step needs to call an LLM for synthesis. This is fundamentally different from embedding (which uses Ollama's `/api/embed` endpoint). Synthesis uses Ollama's `/api/generate` or `/api/chat` endpoint, or an external LLM API.

**Architecture**:

```
tools/compile.go
    └── llm.Synthesizer (interface)
            ├── llm.OllamaSynthesizer  (POST /api/generate)
            └── llm.NoopSynthesizer    (for testing / graceful degradation)
```

**Interface design**:
```go
type Synthesizer interface {
    Synthesize(ctx context.Context, prompt string) (string, error)
    Available() bool
    ModelID() string
}
```

This mirrors the `embed.Embedder` interface pattern. The `Synthesizer` is injected into the `Compile` tool handler, enabling testing with mocks.

**Configuration**: The LLM provider is configured in `.uf/dewey/config.yaml`:
```yaml
compile:
  model: "llama3.2:3b"  # or "opencode" for session model
  ollama_url: "http://localhost:11434"
```

When `model: "opencode"`, the compile MCP tool returns the synthesis prompt as structured output, delegating synthesis to the calling agent. This is the default for session-end auto-compile via `/unleash`.

**Decision**: For the initial implementation, the compile tool always returns the synthesis prompt + clustered learnings as structured output. The calling agent (or CLI) performs the actual synthesis. This avoids the complexity of managing a separate LLM connection and keeps the tool pure (input → output, no side effects beyond file writes).

**Revised approach**: The compile tool does two things:
1. **Cluster**: Read learnings, group by tag + semantic similarity → pure function, no LLM needed
2. **Synthesize**: For each cluster, produce a compiled article → needs LLM

For the MCP tool, the tool returns clusters with a synthesis prompt. The agent performs synthesis and calls back with the result. For the CLI, the tool calls the configured Ollama model directly.

## R6: Compile Tool — Clustering Algorithm

Tag-assisted semantic clustering (FR-007):

1. **Group by tag**: Learnings with the same `tag` value are in the same initial group
2. **Refine by similarity**: Within each tag group, compute pairwise cosine similarity. Split groups where similarity < threshold (e.g., 0.5) into sub-clusters
3. **Cross-tag merge**: Check if any cross-tag pairs have similarity > high threshold (e.g., 0.8). Merge those into the same cluster

**Implementation**: This is a database query + in-memory computation:
```sql
SELECT p.name, p.properties, e.vector, e.chunk_text
FROM pages p
JOIN blocks b ON b.page_name = p.name
JOIN embeddings e ON e.block_uuid = b.uuid
WHERE p.source_id = 'learning'
ORDER BY p.name
```

The clustering is a pure function: `clusterLearnings(learnings []LearningWithEmbedding) []Cluster`. Testable without any external dependencies.

## R7: Category-Aware Resolution

FR-008 defines category-specific merge strategies:

| Category | Resolution Strategy |
|----------|-------------------|
| `decision` | Temporal merge: newer wins, non-contradicted facts carry forward |
| `pattern` | Accumulate: multiple patterns are additive |
| `gotcha` | De-duplicate: same gotcha from different sessions → one entry |
| `context` | Carry forward: unless explicitly contradicted |
| `reference` | Preserve as-is: no modification |

**Implementation**: The resolution strategy is encoded in the synthesis prompt, not in code. The LLM receives the category and applies the appropriate strategy. The prompt template includes category-specific instructions.

For the clustering step (which is code), category affects grouping weight but not the algorithm itself.

## R8: Lint Tool — Quality Checks

FR-016 through FR-019 define four lint checks:

1. **Stale decisions**: `SELECT * FROM pages WHERE source_id = 'learning' AND category = 'decision' AND tier != 'validated' AND created_at < ?` (30 days ago)
2. **Uncompiled learnings**: Compare learning pages against compiled articles. Learnings not referenced by any compiled article are "uncompiled."
3. **Embedding gaps**: `SELECT p.name FROM pages p LEFT JOIN blocks b ON b.page_name = p.name LEFT JOIN embeddings e ON e.block_uuid = b.uuid WHERE e.block_uuid IS NULL`
4. **Semantic contradictions**: For each pair of learnings with similarity > 0.8, check if their conclusions differ (heuristic: different sentiment or opposing keywords)

**`--fix` behavior** (FR-018): Only auto-repairs mechanical issues:
- Embedding gaps: Call `vault.GenerateEmbeddings()` for pages missing embeddings
- Does NOT auto-fix stale decisions or contradictions (requires human/agent judgment)

## R9: Promote Tool — Tier Transitions

FR-023 defines the promote tool:

```sql
UPDATE pages SET tier = 'validated' WHERE name = ? AND tier = 'draft'
```

Simple tier transition. Only `draft` → `validated` is supported. `authored` pages cannot be promoted (they're already the highest trust tier from human sources). `validated` → `authored` is not supported (that would require the page to become a human-authored source).

## R10: Semantic Search Metadata Enrichment

FR-004 and FR-024 require changes to semantic search:

1. **`toSemanticResults()`** in `tools/semantic.go`: Add `created_at` (ISO 8601) and `category` to the result metadata. These are derived from the page's properties JSON and the new `category` column.

2. **`SearchFilters`** in `store/embeddings.go`: Add `Tier string` field. When non-empty, filter by `p.tier = ?` in the SQL query.

3. **`SemanticSearchFilteredInput`** in `types/tools.go`: Add `Tier string` field.

4. **`SemanticSearchResult`** in `types/tools.go`: Add `CreatedAt string` and `Tier string` and `Category string` fields.

## R11: Compiled Article Output Format

Compiled articles are markdown files in `.uf/dewey/compiled/`:

```
.uf/dewey/compiled/
├── _index.md              # Auto-generated index of all compiled articles
├── authentication.md      # Compiled article for "authentication" topic
├── database-patterns.md   # Compiled article for "database-patterns" topic
└── deployment-gotchas.md  # Compiled article for "deployment-gotchas" topic
```

Each compiled article follows a standard format:
```markdown
---
tier: draft
compiled_at: 2026-04-10T14:30:00Z
sources: ["authentication-1", "authentication-2", "authentication-3"]
---

# Authentication

## Current State

[Merged truth from all learnings, with contradictions resolved by recency]

## History

- **authentication-1** (2026-03-15): Use Option A for auth. Timeout 30s.
- **authentication-2** (2026-03-20): Switch to Option B due to rate limiting.
- **authentication-3** (2026-04-01): Increase timeout to 60s per user feedback.
```

The compiled articles are indexed as regular pages with `source_id = "compiled"` and `tier = "draft"`. They are searchable via semantic search alongside other content.

## R12: Session-End Auto-Compile Integration

The `/unleash` command file (`.opencode/command/unleash.md`) needs a documentation update to add the compile trigger to the retrospective step. This is NOT a code change — it's an agent instruction file update.

The compile MCP tool supports incremental compilation: when called with specific learning IDs, it only processes those learnings against existing compiled articles. This is the mode used by session-end auto-compile.

**Non-blocking behavior** (FR-014): The `/unleash` instruction wraps the compile call in a try/catch pattern — if compilation fails, the retrospective continues. The compile tool itself returns an error result (not a Go error), so the agent can handle it gracefully.

## R13: Backward Compatibility Analysis

**Breaking changes**:
- `store_learning` API: `tags` (plural, optional) → `tag` (singular, required). Agents calling with the old `tags` parameter will get an error. **Mitigation**: The tool handler checks for both `tags` and `tag` in the input. If `tags` is provided but `tag` is not, use the first tag from the comma-separated list. If neither is provided, default to `"general"`.

**Non-breaking changes**:
- Schema migration adds columns with defaults — existing queries continue to work
- New MCP tools don't affect existing tools
- Semantic search result metadata additions are additive (new fields, no removed fields)
- Compiled articles are new pages — don't interfere with existing pages

**Existing test impact**:
- `tools/learning_test.go`: Tests need updating for new `tag` parameter and `{tag}-{sequence}` identity
- `tools/semantic_test.go`: Tests need updating for new metadata fields in results
- `store/migrate_test.go`: New migration test for v1 → v2
- All other tests: No changes expected (schema migration is backward-compatible)
