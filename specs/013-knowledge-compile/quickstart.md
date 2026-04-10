# Phase 1 Quickstart: Knowledge Compilation & Temporal Intelligence

**Branch**: `013-knowledge-compile` | **Date**: 2026-04-10

## Architecture Overview

This feature transforms Dewey's learning store into an event-sourced knowledge system. The architecture follows the Event Sourcing pattern: raw learnings are the append-only event log, compiled articles are the materialized view.

### Event Sourcing Model

```
┌─────────────────────────────────────────────────────────┐
│                    Event Log (Learnings)                 │
│  ┌──────────────┐ ┌──────────────┐ ┌──────────────┐    │
│  │ auth-1       │ │ auth-2       │ │ auth-3       │    │
│  │ tag: auth    │ │ tag: auth    │ │ tag: auth    │    │
│  │ cat: decision│ │ cat: decision│ │ cat: context │    │
│  │ tier: draft  │ │ tier: draft  │ │ tier: draft  │    │
│  │ Use Option A │ │ Switch to B  │ │ Timeout 60s  │    │
│  └──────────────┘ └──────────────┘ └──────────────┘    │
└─────────────────────────────────────────────────────────┘
                          │
                    dewey compile
                          │
                          ▼
┌─────────────────────────────────────────────────────────┐
│              Materialized View (Compiled Articles)       │
│  ┌─────────────────────────────────────────────────┐    │
│  │ .uf/dewey/compiled/authentication.md            │    │
│  │ ## Current State                                │    │
│  │ Use Option B (changed from A). Timeout: 60s.    │    │
│  │ ## History                                      │    │
│  │ auth-1: Use Option A, 30s timeout               │    │
│  │ auth-2: Switch to Option B                      │    │
│  │ auth-3: Increase timeout to 60s                 │    │
│  └─────────────────────────────────────────────────┘    │
└─────────────────────────────────────────────────────────┘
```

### Component Interaction

```
                    ┌──────────────┐
                    │   Agent      │
                    └──────┬───────┘
                           │ MCP tool calls
              ┌────────────┼────────────┐
              ▼            ▼            ▼
     ┌────────────┐ ┌───────────┐ ┌──────────┐
     │store_learn │ │  compile  │ │   lint   │
     │  (tools/)  │ │ (tools/)  │ │ (tools/) │
     └─────┬──────┘ └─────┬─────┘ └────┬─────┘
           │              │             │
           ▼              ▼             ▼
     ┌─────────────────────────────────────────┐
     │           store.Store (SQLite)           │
     │  pages: tier, category, created_at       │
     │  blocks, embeddings, links               │
     └─────────────────────────────────────────┘
           │              │
           ▼              ▼
     ┌──────────┐  ┌──────────────────┐
     │ embed/   │  │ llm/ (synthesis) │
     │ (Ollama  │  │ (Ollama /api/    │
     │ /api/    │  │  generate)       │
     │  embed)  │  │                  │
     └──────────┘  └──────────────────┘
```

## Key Design Decisions

### D1: Schema Migration v1 → v2

**Decision**: Add two columns to the `pages` table via `ALTER TABLE`:
- `tier TEXT DEFAULT 'authored'` — trust tier for contamination separation
- `category TEXT` — learning category for category-aware resolution

The existing `created_at INTEGER` column (Unix milliseconds) is sufficient for temporal ordering. ISO 8601 format is derived at the tool response layer, not stored redundantly.

**Migration steps**:
1. `ALTER TABLE pages ADD COLUMN tier TEXT DEFAULT 'authored'`
2. `ALTER TABLE pages ADD COLUMN category TEXT`
3. `UPDATE pages SET tier = 'draft' WHERE source_id = 'learning'`
4. `UPDATE pages SET tier = 'draft' WHERE source_id = 'compiled'`
5. `CREATE INDEX IF NOT EXISTS idx_pages_tier ON pages(tier)`
6. Update `schema_version` to `2`

**Alternative rejected**: Storing `tier` in the properties JSON — not queryable at the SQL level, which is needed for `SearchSimilarFiltered` tier filtering.

### D2: `store_learning` API Change — `tags` → `tag`

**Decision**: The `StoreLearningInput` struct changes:
- Remove: `Tags string` (plural, optional, comma-separated)
- Add: `Tag string` (singular, required — topic namespace)
- Add: `Category string` (optional enum: decision/pattern/gotcha/context/reference)

**Backward compatibility**: The tool handler checks for the old `tags` field. If `tags` is provided but `tag` is not, the first tag from the comma-separated list is used. If neither is provided, `tag` defaults to `"general"`.

**Identity**: Page name changes from `learning/{timestamp}` to `learning/{tag}-{sequence}`. The sequence is determined by counting existing pages with the same tag prefix.

### D3: `{tag}-{sequence}` Identity

**Decision**: The learning identity is encoded in the page name: `learning/{tag}-{sequence}`.

**Sequence determination**:
```sql
SELECT COUNT(*) FROM pages 
WHERE source_id = 'learning' 
AND name LIKE 'learning/' || ? || '-%'
```
New sequence = count + 1.

**Tag normalization**: Lowercase, trim whitespace, replace spaces with hyphens, strip non-alphanumeric characters (except hyphens).

**Alternative rejected**: UUID-based identity — not human-readable, not addressable by topic.

### D4: LLM Synthesis — Prompt-Based Delegation

**Decision**: The compile MCP tool does NOT call an LLM directly. Instead, it:
1. Reads all learnings from the store
2. Clusters them by tag + semantic similarity (pure function)
3. Returns structured output: clusters + synthesis prompts + category-aware instructions

The calling agent performs the actual synthesis and writes the compiled articles. This design:
- Avoids managing a separate LLM connection in the MCP server
- Works with any LLM (the agent's own model, regardless of provider)
- Is fully testable (no LLM mock needed for the tool itself)
- Follows the spec's insight: "for session-end compile, the agent IS the LLM"

For the CLI `dewey compile`, the command calls the configured Ollama model via the `llm.Synthesizer` interface. The CLI is the only path that needs a direct LLM connection.

### D5: Compile Tool — Two-Phase Architecture

**Decision**: The compile tool has two modes:

1. **MCP tool mode** (`compile`): Returns clusters + prompts. The agent synthesizes and calls `compile_write` to persist articles. This is a read-only operation.

2. **CLI mode** (`dewey compile`): Calls the Ollama generation model, synthesizes articles, and writes them to `.uf/dewey/compiled/`. This is a write operation.

**Revised decision**: Simplify to a single mode. The compile tool:
1. Clusters learnings (pure function)
2. Generates synthesis prompts with category-aware instructions
3. Calls the injected `llm.Synthesizer` for each cluster
4. Writes compiled articles to `.uf/dewey/compiled/`
5. Indexes compiled articles into the store

When `Synthesizer` is nil (no LLM configured), the tool returns the clusters and prompts as structured output, allowing the agent to perform synthesis externally.

### D6: Compiled Article Lifecycle

**Decision**: Compiled articles are ephemeral. A full `dewey compile` deletes the entire `.uf/dewey/compiled/` directory and rebuilds from scratch. This ensures deterministic output (FR-028) and avoids stale article accumulation.

Incremental compilation (for session-end auto-compile) merges new learnings into existing articles without a full rebuild. If the incremental result would change the article taxonomy (e.g., a new topic cluster), a full rebuild is triggered instead.

**Storage**: Compiled articles are indexed as pages with `source_id = "compiled"` and `tier = "draft"`. They are searchable via semantic search.

### D7: Lint Tool — Check Categories

**Decision**: The lint tool runs four independent checks:

| Check | Query | Auto-fixable |
|-------|-------|-------------|
| Stale decisions | `category = 'decision' AND tier != 'validated' AND created_at < 30d ago` | No |
| Uncompiled learnings | Learnings not referenced by any compiled article | No |
| Embedding gaps | Pages with blocks but no embeddings | Yes (`--fix`) |
| Semantic contradictions | Learning pairs with similarity > 0.8 and same tag | No |

Each check returns a structured finding with severity, description, and remediation suggestion.

### D8: Promote Tool — Simple Tier Transition

**Decision**: The promote tool changes a page's `tier` from `draft` to `validated`. Only `draft` → `validated` is supported. The tool accepts a page name and validates:
1. The page exists
2. The page's current tier is `draft`
3. Updates `tier` to `validated`

No other tier transitions are supported. `authored` pages cannot be promoted (already highest trust). `validated` cannot be demoted (that would require a separate `demote` tool, out of scope).

### D9: Semantic Search Metadata Enrichment

**Decision**: Extend `SemanticSearchResult` with three new fields:
- `created_at string` — ISO 8601 timestamp (derived from page's `created_at` Unix ms)
- `tier string` — trust tier (authored/validated/draft)
- `category string` — learning category (decision/pattern/gotcha/context/reference), empty for non-learning pages

Extend `SearchFilters` with:
- `Tier string` — when non-empty, adds `AND p.tier = ?` to the filtered query

Extend `SemanticSearchFilteredInput` with:
- `Tier string` — exposed as `tier` parameter in the MCP tool schema

### D10: `/unleash` Integration

**Decision**: Update `.opencode/command/unleash.md` to add a compile trigger after the retrospective stores learnings. The instruction wraps the compile call in error handling so compilation failure is non-blocking (FR-014).

This is a documentation change, not a code change. The compile MCP tool is called by the agent during the `/unleash` flow.

## Contracts

See `contracts/` directory for detailed API contracts:
- `contracts/store-learning.md` — Modified `store_learning` tool
- `contracts/compile-tool.md` — New `compile` tool
- `contracts/lint-tool.md` — New `lint` tool
- `contracts/promote-tool.md` — New `promote` tool
- `contracts/schema-migration.md` — Schema v1 → v2
- `contracts/llm-interface.md` — LLM synthesis interface

## Coverage Strategy

### What to Test

1. **Schema migration v1 → v2**: Verify columns are added, defaults applied, learning pages get `tier = 'draft'`, index created
2. **`store_learning` with tag/category**: Verify `{tag}-{sequence}` identity, `created_at` in properties, `tier = 'draft'`, backward compatibility with old `tags` parameter
3. **Compile clustering**: Verify tag-based grouping, semantic similarity refinement, category-aware prompt generation
4. **Compile article output**: Verify markdown format, current-state section, history section, `_index.md` generation
5. **Lint checks**: Verify each of the 4 checks independently with fixture data
6. **Lint --fix**: Verify embedding regeneration for pages with gaps
7. **Promote**: Verify `draft` → `validated` transition, rejection of non-draft pages
8. **Semantic search metadata**: Verify `created_at`, `tier`, `category` in results
9. **Tier filtering**: Verify `semantic_search_filtered` with `tier` parameter
10. **Backward compatibility**: Verify existing tests pass without modification (except learning tests)

### How to Test

- **Store tests** (`store/migrate_test.go`): In-memory SQLite, verify schema changes
- **Tool tests** (`tools/*_test.go`): Mock store with pre-populated data, mock embedder, mock synthesizer
- **Integration tests**: End-to-end flow: store learnings → compile → search compiled articles
- **CLI tests** (`cli_test.go`): Verify new subcommands parse flags and produce expected output

### Coverage Targets

- All new code paths must have contract-level tests
- Existing tests must continue to pass
- CRAP score for all functions must stay below 48 (CI threshold)
- CRAP score for new functions should target < 18 (Gaze crapload threshold)
- Contract coverage for new packages: ≥ 70% (CI threshold)
