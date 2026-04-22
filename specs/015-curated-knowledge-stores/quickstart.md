# Quickstart: Curated Knowledge Stores

**Branch**: `015-curated-knowledge-stores` | **Date**: 2026-04-21

## Minimal Viable Path

The fastest path to a working curated knowledge store:

### Step 1: File-Backed Learnings (US1)

**Goal**: Every `store_learning` call produces a markdown file alongside the SQLite record.

1. Add `vaultPath string` field to `Learning` struct in `tools/learning.go`
2. Update `NewLearning()` to accept `vaultPath`
3. After `InsertPage()` succeeds, write markdown file to `.uf/dewey/learnings/{tag}-{seq}.md`
4. Update `server.go` to pass `cfg.vaultPath` to `NewLearning()`
5. Add re-ingestion in `main.go` — scan learnings dir on startup, re-ingest orphans

**Test**: Store a learning → verify `.md` file exists → delete `graph.db` → restart → verify learning is back.

### Step 2: Knowledge Store Config (US2)

**Goal**: Parse `knowledge-stores.yaml` and validate store definitions.

1. Create `curate/config.go` with `StoreConfig` struct and `LoadKnowledgeStoresConfig()`
2. Add `knowledge-stores.yaml` scaffold to `dewey init`
3. Validate source IDs against `sources.yaml`

**Test**: Create config with valid/invalid stores → verify parsing and validation.

### Step 3: Curation Pipeline (US3 + US4)

**Goal**: Extract knowledge from indexed content using LLM.

1. Create `curate/curate.go` with `Pipeline` struct
2. Implement `CurateStore()` — reads pages, builds prompt, calls LLM, writes files
3. Add `curate` MCP tool in `tools/curate.go`
4. Add `dewey curate` CLI command in `cli.go`

**Test**: Index test content → run curation with `NoopSynthesizer` → verify knowledge files.

### Step 4: Curated Tier (US6)

**Goal**: Curated content is filterable by tier.

1. Set `tier: "curated"` on pages from `knowledge-*` sources
2. Verify `semantic_search_filtered(tier: "curated")` works

**Test**: Curate content → filter by tier → verify only curated content returned.

### Step 5: Background + Lint + Auto-Index (US5, US7, US8)

**Goal**: Continuous curation, quality reporting, automatic searchability.

1. Background goroutine in `main.go`
2. Lint checks in `tools/lint.go`
3. Auto-register knowledge store dirs as disk sources

**Test**: Start serve → add content → wait → verify curation runs automatically.

## What Can Be Deferred

- **Temporal resolution** (FR-011): Can start with "newer wins" as a simple rule. Sophisticated temporal merge can be enhanced later.
- **Multi-source aggregation** (US3 AS3): Can start with per-document extraction. Cross-document aggregation can be a follow-up.
- **Curation interval tuning**: Default 10m is reasonable. Per-store tuning is already in the config schema.

## What Cannot Be Deferred

- **File-backed learnings** (US1): Foundation for everything else.
- **Source traceability** (FR-010): Core value proposition — every fact traces to its source.
- **Quality flags** (FR-013-016): Part of the LLM prompt — no extra implementation cost.
- **Mutex integration** (FR-020): Safety requirement — concurrent curation/indexing corrupts data.
