# Research: Curated Knowledge Stores

**Branch**: `015-curated-knowledge-stores` | **Date**: 2026-04-21
**Spec**: [spec.md](spec.md) | **Plan**: [plan.md](plan.md)

## R1: File-Backed Learning Persistence Pattern

### Current State

`tools/learning.go` (206 lines) implements the `store_learning` MCP tool. The `StoreLearning` method:
1. Validates input (information required, category optional)
2. Resolves tag via `resolveTag()` (tag > tags > "general")
3. Gets next sequence via `store.NextLearningSequence(tag)`
4. Builds identity `{tag}-{seq}` and page name `learning/{identity}`
5. Inserts page with `source_id = "learning"`, `tier = "draft"`
6. Parses content into blocks via `vault.ParseDocument()`
7. Persists blocks via `vault.PersistBlocks()`
8. Generates embeddings if available
9. Returns JSON result

### Dual-Write Design

The dual-write to markdown should happen after step 5 (store insert succeeds) and before step 9 (response). If the file write fails, the learning is still in the database — log a warning but don't fail the operation.

**File format**:
```markdown
---
tag: authentication
category: decision
created_at: "2026-04-21T10:30:00Z"
identity: authentication-3
tier: draft
---

Sarah confirmed we need OAuth2 + SAML for enterprise. The team
agreed to implement OAuth2 first and add SAML in Q3.
```

**Directory**: `.uf/dewey/learnings/` — created on first write if it doesn't exist.

### Re-Ingestion Design

On `dewey serve` startup, after the store is opened but before background indexing:

1. Resolve the learnings directory: `{vaultPath}/.uf/dewey/learnings/`
2. If directory doesn't exist, skip (no learnings to re-ingest)
3. Walk `.md` files in the directory
4. For each file:
   a. Parse YAML frontmatter to extract `identity`
   b. Check if `learning/{identity}` page exists in store
   c. If not, re-ingest: insert page, parse blocks, persist blocks, generate embeddings
   d. Preserve original `created_at`, `tag`, `category` from frontmatter (FR-004)

**Dependency**: The `Learning` struct needs a `vaultPath` field. This matches the `Compile` struct pattern (`tools/compile.go` line 63).

### Constructor Change

```go
// Before:
func NewLearning(e embed.Embedder, s *store.Store) *Learning

// After:
func NewLearning(e embed.Embedder, s *store.Store, vaultPath string) *Learning
```

This is a breaking change to the constructor signature. Update call sites:
- `server.go` line 112: `tools.NewLearning(cfg.embedder, cfg.store)` → `tools.NewLearning(cfg.embedder, cfg.store, cfg.vaultPath)`

## R2: Knowledge Store Configuration Schema

### YAML Structure

Following the `source/config.go` pattern (SourcesFile → []SourceConfig):

```yaml
# .uf/dewey/knowledge-stores.yaml
stores:
  - name: team-decisions
    sources: [disk-meetings, disk-slack-export]
    path: .uf/dewey/knowledge/team-decisions
    curate_on_index: true
    curation_interval: 10m
```

### Validation Rules

1. `name` is required and must be unique across stores
2. `sources` is required and must be non-empty (skip store with warning if empty — FR-006 AS3)
3. `path` defaults to `.uf/dewey/knowledge/{name}` if not specified
4. `curation_interval` defaults to `10m` if not specified. Parsed via `source.ParseRefreshInterval()` (reuse existing duration parser)
5. Source IDs in `sources` are validated against `sources.yaml` at load time — missing sources log a warning but don't fail (FR-006 AS4)

### Config File Location

`.uf/dewey/knowledge-stores.yaml` — same directory as `sources.yaml` and `config.yaml`.

### Scaffolding in `dewey init`

Add to `newInitCmd()` in `cli.go`, after `sources.yaml` creation:

```go
knowledgeStoresPath := filepath.Join(deweyDir, "knowledge-stores.yaml")
knowledgeStoresContent := `# Knowledge store configuration
# Each store curates knowledge from indexed sources.
# Uncomment and customize the example below.

# stores:
#   - name: team-decisions
#     sources: [disk-local]
#     # path: .uf/dewey/knowledge/team-decisions  # default
#     # curate_on_index: false                     # default
#     # curation_interval: 10m                     # default
`
```

## R3: Curation Pipeline Architecture

### Pipeline Flow

```
1. Load store config
2. For each store:
   a. Load curation state (checkpoint)
   b. Query store for pages from configured sources
   c. Filter to pages updated since last checkpoint
   d. For each page:
      i.  Retrieve blocks from store
      ii. Concatenate into document text
   e. Build LLM prompt with all documents
   f. Call LLM (or return prompt if no synthesizer)
   g. Parse LLM response (JSON array of extracted knowledge)
   h. For each extracted piece:
      i.   Write markdown file to store path
      ii.  Track for auto-indexing
   i. Update curation checkpoint
3. Auto-index knowledge store directories
```

### LLM Prompt Design

The prompt instructs the LLM to extract structured knowledge:

```
You are a knowledge curator. Analyze the following source documents and extract
key decisions, facts, patterns, and context. For each piece of knowledge:

1. Assign a topic tag (lowercase, hyphenated)
2. Categorize as: decision, pattern, gotcha, context, or reference
3. Assess confidence: high (explicit, no contradictions), medium (single source),
   low (implied or contradicted), flagged (missing critical info)
4. Identify quality issues:
   - missing_rationale: decision without explanation
   - implied_assumption: unstated assumption
   - incongruent: contradicts another source
   - unsupported_claim: fact without source evidence
5. Include source traceability: which document, which section

Output as JSON array:
[{
  "tag": "authentication",
  "category": "decision",
  "confidence": "high",
  "quality_flags": [],
  "sources": [{"source_id": "disk-meetings", "document": "2026-04-sprint-review", "section": "Auth Discussion"}],
  "content": "Team decided to implement OAuth2 for all API endpoints..."
}]

Source documents:
---
[document content here]
---
```

### MCP Tool vs CLI Mode

- **MCP tool** (`tools/curate.go`): When `synth` is nil (MCP server mode), return the prompt as structured output for the calling agent. Same pattern as `tools/compile.go` with nil synthesizer.
- **CLI command** (`dewey curate`): Create an `OllamaSynthesizer` and pass it to the pipeline. Same pattern as `newCompileCmd()` in `cli.go`.

## R4: Incremental Curation Checkpoints

### State File Format

`.uf/dewey/knowledge/{store-name}/.curation-state.json`:

```json
{
  "last_curated_at": "2026-04-21T10:30:00Z",
  "source_checkpoints": {
    "disk-meetings": "2026-04-21T10:30:00Z",
    "disk-slack-export": "2026-04-20T15:00:00Z"
  }
}
```

### Incremental Query

```go
// Query pages from a source updated after the checkpoint
pages, err := store.ListPagesBySourceUpdatedAfter(sourceID, checkpoint)
```

This requires a new store method:

```go
func (s *Store) ListPagesBySourceUpdatedAfter(sourceID string, after int64) ([]*Page, error) {
    rows, err := s.db.Query(`
        SELECT ... FROM pages
        WHERE source_id = ? AND updated_at > ?
        ORDER BY updated_at`, sourceID, after)
    ...
}
```

### Checkpoint Update

After successful curation of a source's pages, update the checkpoint:

```go
state.SourceCheckpoints[sourceID] = time.Now()
state.LastCuratedAt = time.Now()
// Write to .curation-state.json
```

## R5: Background Curation Goroutine

### Integration Point

In `main.go` `executeServe()`, after the background indexing goroutine launch (line 292-307):

```go
// Launch background curation goroutines (one per store with curate_on_index or curation_interval).
if len(knowledgeStores) > 0 && !noEmbeddings {
    go backgroundCuration(ctx, indexMu, indexReady, knowledgeStores, persistentStore, embedder, vaultPath)
}
```

### Goroutine Lifecycle

```go
func backgroundCuration(ctx context.Context, mu *sync.Mutex, ready *atomic.Bool, stores []curate.StoreConfig, s *store.Store, e embed.Embedder, vaultPath string) {
    // Wait for indexing to complete before first curation.
    for !ready.Load() {
        select {
        case <-ctx.Done():
            return
        case <-time.After(1 * time.Second):
        }
    }

    // Create per-store tickers.
    for _, storeCfg := range stores {
        interval := storeCfg.ParsedInterval()
        if interval == 0 {
            continue // No background curation for this store.
        }

        go func(cfg curate.StoreConfig) {
            ticker := time.NewTicker(interval)
            defer ticker.Stop()

            for {
                select {
                case <-ctx.Done():
                    return
                case <-ticker.C:
                    if !mu.TryLock() {
                        logger.Debug("skipping curation — indexing in progress", "store", cfg.Name)
                        continue
                    }
                    // Run incremental curation.
                    pipeline := curate.NewPipeline(s, nil, e, vaultPath) // nil synth for background
                    if err := pipeline.CurateStore(ctx, cfg); err != nil {
                        logger.Error("background curation failed", "store", cfg.Name, "err", err)
                    }
                    mu.Unlock()
                }
            }
        }(storeCfg)
    }
}
```

**Key design decisions**:
- `TryLock()` instead of `Lock()` — if indexing is in progress, skip this curation cycle rather than blocking.
- One goroutine per store — stores with different intervals run independently.
- `nil` synthesizer for background mode — background curation without a local LLM is a no-op (logs info and skips). The LLM must be available for background curation to produce results.

### Synthesizer for Background Curation

Background curation needs a synthesizer. Options:
1. **Ollama** (if available): Create `llm.NewOllamaSynthesizer()` using the same model env vars as `dewey compile`.
2. **None**: If Ollama is unavailable, background curation logs "LLM unavailable — skipping background curation" and waits for next interval.

This matches the graceful degradation principle — background curation is a best-effort enhancement, not a hard requirement.

## R6: Curated Trust Tier — No Migration Needed

### Verification

The `tier` column in the `pages` table is `TEXT DEFAULT 'authored'` (store/migrate.go line 93). It accepts any string value — no CHECK constraint, no ENUM. Adding `curated` as a value requires zero schema changes.

### Tier Hierarchy (for documentation)

```
authored > curated > validated > draft
```

- `authored`: Human-written content (disk, GitHub, web, code sources)
- `curated`: Machine-extracted from sources with traceability
- `validated`: Agent content promoted by human review
- `draft`: Agent-stored learnings (unreviewed)

### Filtering Verification

`filterResultsByTier()` in `tools/semantic.go` (line 322) does `page.Tier == tier` — string equality. Works immediately for `curated`.

## R7: Auto-Indexing Knowledge Store Directories

### Registration Flow

After curation writes files:

1. Check if source `knowledge-{store-name}` exists in the store's `sources` table.
2. If not, register it:
   ```go
   store.InsertSource(&store.SourceRecord{
       ID:   "knowledge-" + storeName,
       Type: "disk",
       Name: "knowledge-" + storeName,
       Status: "active",
   })
   ```
3. Index the directory using the existing `indexDocuments()` pipeline from `cli.go`.
4. Set `tier: "curated"` on all pages from `knowledge-*` sources.

### Tier Assignment During Indexing

In `indexDocuments()` (cli.go line 974), after page upsert:

```go
// Set tier for knowledge store sources.
if strings.HasPrefix(sourceID, "knowledge-") {
    page.Tier = "curated"
}
```

This ensures all auto-indexed knowledge store content gets the `curated` tier.

### Preventing Purge

The `purgeOrphanedSources()` function (cli.go line 941) deletes pages for sources not in `sources.yaml`. Knowledge store sources are auto-registered, not in `sources.yaml`. To prevent purge:

- Option A: Add knowledge store sources to `sources.yaml` dynamically — complex, modifies user file.
- Option B: Skip purge for `knowledge-*` prefixed sources — simple, explicit.

**Decision**: Option B. Add a check in `purgeOrphanedSources()`:

```go
if strings.HasPrefix(src.ID, "knowledge-") {
    continue // Auto-registered knowledge store source — don't purge.
}
```
