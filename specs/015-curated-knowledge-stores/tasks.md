# Tasks: Curated Knowledge Stores

**Input**: Design documents from `/specs/015-curated-knowledge-stores/`
**Prerequisites**: plan.md (required), spec.md (required), research.md, quickstart.md, contracts/curate-package.md, contracts/mcp-tools.md, contracts/cli-commands.md

**Tests**: Included — this spec has 28 FRs across 8 user stories; tests are required per constitution (Observable Quality, Testability).

**Organization**: Tasks are grouped into 7 phases following the plan.md implementation phases. Phases 1–4 are sequential (each builds on the prior). Phases 5–6 depend on Phase 3. Phase 7 depends on all prior phases.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (e.g., US1, US2)
- Include exact file paths in descriptions

## Path Conventions

- **Project root**: Go package layout at repository root
- **New package**: `curate/` (config parsing + curation pipeline)
- **Modified packages**: `tools/`, `store/`, `types/`
- **Modified root files**: `main.go`, `server.go`, `cli.go`, `slash_commands.go`

---

## Phase 1: File-Backed Learning Persistence (US1 — Foundation)

**Purpose**: Every `store_learning` call dual-writes to SQLite AND a markdown file. Orphaned markdown files are re-ingested on startup. This is the foundation everything else builds on.

**Goal**: Learnings survive `graph.db` deletion — zero knowledge loss.

**Independent Test**: Store 3 learnings → verify `.md` files exist → delete `graph.db` → restart → verify all 3 learnings are re-ingested from markdown files and appear in search results.

### Implementation

- [x] T001 [US1] Add `vaultPath string` field to `Learning` struct and update `NewLearning()` constructor signature in `tools/learning.go` — change from `NewLearning(e, s)` to `NewLearning(e, s, vaultPath)`. Update call site in `server.go` to pass `cfg.vaultPath`. (FR-001)
- [x] T002 [US1] Implement markdown dual-write in `tools/learning.go` — after `InsertPage()` succeeds, write markdown file to `.uf/dewey/learnings/{tag}-{seq}.md` with YAML frontmatter (`tag`, `category`, `created_at`, `identity`, `tier`) and learning content as body. Create learnings directory if it doesn't exist. Log warning on file write failure but don't fail the operation. Add `file_path` field to the JSON response. (FR-001, FR-002)
- [x] T003 [US1] Implement learning re-ingestion on startup in `main.go` — after store is opened but before background indexing starts, scan `.uf/dewey/learnings/` for `.md` files. For each file: parse YAML frontmatter to extract `identity`, check if `learning/{identity}` page exists in store, if not re-ingest by calling store insert + block persist + embedding generation pipeline. Preserve original `created_at`, `tag`, `category` from frontmatter. (FR-003, FR-004)

### Tests

- [x] T004 [P] [US1] Add tests for file-backed learning persistence in `tools/learning_test.go` — test dual-write creates markdown file with correct frontmatter, test file content matches learning input, test file write failure doesn't fail the store operation, test `file_path` appears in response JSON. Use `t.TempDir()` for filesystem isolation.
- [x] T005 [P] [US1] Add tests for learning re-ingestion in `tools/learning_test.go` (or a new `main_test.go` helper) — test orphaned markdown files are re-ingested on startup, test re-ingestion preserves original `created_at`/`tag`/`category`/`identity`, test already-ingested files are skipped, test missing learnings directory is handled gracefully.

**Checkpoint**: `store_learning` produces markdown files. Deleting `graph.db` and restarting recovers all learnings. Run `go test -race -count=1 ./tools/...` — all tests pass.

---

## Phase 2: Knowledge Store Configuration (US2)

**Purpose**: Parse `knowledge-stores.yaml` and validate store definitions. Scaffold the config file via `dewey init`.

**Goal**: Users can define named knowledge stores that map sources to output directories.

**Independent Test**: Create a `knowledge-stores.yaml` with valid/invalid stores → verify parsing, defaults, and validation.

### Implementation

- [x] T006 [US2] Create `curate/config.go` — define `StoreConfig` struct (per contracts/curate-package.md), implement `LoadKnowledgeStoresConfig(path string) ([]StoreConfig, error)` that reads and parses YAML, returns `(nil, nil)` if file doesn't exist. Apply defaults: `Path` → `.uf/dewey/knowledge/{Name}`, `CurationInterval` → `"10m"`. Implement `ResolveStorePath()` and `ParseCurationInterval()`. (FR-005, FR-006)
- [x] T007 [US2] Add config validation in `curate/config.go` — validate `Name` is non-empty and unique, `Sources` is non-empty (skip with warning if empty), source IDs exist in sources.yaml (log warning for missing, don't fail). Validate `CurationInterval` parses as a valid duration. (FR-006)
- [x] T008 [US2] Update `dewey init` in `cli.go` — after `sources.yaml` creation, scaffold `.uf/dewey/knowledge-stores.yaml` with commented-out example store. Follow existing idempotency pattern (don't overwrite if file exists). (FR-007)

### Tests

- [x] T009 [P] [US2] Add tests for config parsing in `curate/config_test.go` — test valid YAML parsing, test missing file returns `(nil, nil)`, test malformed YAML returns error, test default values applied (path, interval), test duplicate store names rejected, test empty sources skipped with no error, test `ResolveStorePath()` with absolute/relative/empty paths, test `ParseCurationInterval()` with valid/invalid/empty strings.

**Checkpoint**: `curate.LoadKnowledgeStoresConfig()` correctly parses and validates config. `dewey init` scaffolds the file. Run `go test -race -count=1 ./curate/...` — all tests pass.

---

## Phase 3: Curation Pipeline (US3, US4 — Core Intelligence)

**Purpose**: The curation engine that reads indexed content, uses LLM to extract structured knowledge with quality analysis and confidence scoring, and writes curated markdown files.

**Goal**: `dewey curate` produces knowledge files with source traceability, quality flags, and confidence scores.

**Independent Test**: Index a source with meeting notes → run `dewey curate` → verify knowledge files appear with correct tags, categories, source references, quality flags, and confidence scores.

### Pipeline Core

- [x] T010 [US3] Create `curate/curate.go` — define `Pipeline` struct, `DocumentContent`, `CurationResult`, `KnowledgeFile`, `QualityFlag`, `SourceRef`, `CurationState` types (per contracts/curate-package.md). Implement `NewPipeline(s, synth, e, vaultPath)` constructor. (FR-008, FR-009)
- [x] T011 [US3] Implement source content reading in `curate/curate.go` — add method to load indexed documents for configured source IDs from the store. Query pages by `source_id`, retrieve blocks for each page, concatenate into `DocumentContent` structs. (FR-008)
- [x] T012 [US3] Add `ListPagesBySourceUpdatedAfter(sourceID string, after int64) ([]*types.PageEntity, error)` method to `store/store.go` — query pages table filtered by `source_id` and `updated_at > after`. Returns pages ordered by `updated_at`. (Supports FR-019 incremental curation)
- [x] T013 [US3] Implement LLM extraction prompt in `curate/curate.go` — `BuildExtractionPrompt(documents []DocumentContent) string` that constructs the structured prompt instructing the LLM to extract decisions/facts/patterns with tags, categories, confidence scores, quality flags, and source traceability. Include JSON output format specification and examples. (FR-009, FR-010)
- [x] T014 [US4] Implement quality analysis within the extraction prompt in `curate/curate.go` — the prompt instructs the LLM to detect: `missing_rationale` (decisions without explanation), `implied_assumption` (unstated assumptions), `incongruent` (cross-source contradictions with temporal resolution), `unsupported_claim` (facts without evidence). Each flag includes detail, source references, and optional resolution. (FR-013, FR-014, FR-015)
- [x] T015 [US4] Implement confidence scoring logic in `curate/curate.go` — `high` (explicit, no contradictions, multiple sources agree), `medium` (explicit but single-source), `low` (implied or contradictions exist), `flagged` (missing critical info or unresolvable contradictions). Part of the LLM prompt and validated in `ParseExtractionResponse()`. (FR-016)
- [x] T016 [US3] Implement `ParseExtractionResponse(response string) ([]KnowledgeFile, error)` in `curate/curate.go` — parse LLM JSON response into `KnowledgeFile` structs. Validate required fields (tag, category, confidence, sources non-empty). Handle malformed JSON gracefully. (FR-009)
- [x] T017 [US3] Implement `WriteKnowledgeFile(file KnowledgeFile, storePath string, seq int) (string, error)` in `curate/curate.go` — write markdown file with YAML frontmatter to `{storePath}/{tag}-{seq}.md`. Create directory if needed. Apply temporal resolution for contradictions (newer wins). (FR-010, FR-011, FR-012)

### Incremental Curation

- [x] T018 [US3] Implement curation checkpoint in `curate/curate.go` — `LoadCurationState(storePath)` reads `.curation-state.json` from store directory (returns zero-value if missing), `SaveCurationState(state, storePath)` writes checkpoint after successful curation. Track `last_curated_at` and per-source timestamps in `source_checkpoints`. (FR-019)
- [x] T019 [US3] Implement `CurateStore()` and `CurateStoreIncremental()` in `curate/curate.go` — full pipeline: load checkpoint → query pages → filter by checkpoint (incremental) or process all (full) → build prompt → call LLM (or return prompt if synth is nil) → parse response → write files → update checkpoint. Return `CurationResult`. (FR-008, FR-019)

### MCP Tool + CLI + Slash Command

- [x] T020 [US3] Create `tools/curate.go` — MCP tool handler for `curate`. Define `Curate` struct with `store`, `embedder`, `synth`, `vaultPath`, `indexMutex` fields. Implement `NewCurate()` constructor. Handle two modes: with synthesizer (run pipeline, return results) and without synthesizer (return extraction prompts). Acquire mutex before curation, release after. Handle error cases per contracts/mcp-tools.md. (FR-008)
- [x] T021 [US3] Add `CurateInput` type to `types/tools.go` — `Store string`, `Incremental *bool` fields with JSON tags per contracts/mcp-tools.md input schema.
- [x] T022 [US3] Register curate MCP tool in `server.go` — add `registerCurateTools()` function following existing registration patterns. Wire `tools.NewCurate()` with `cfg.store`, `cfg.embedder`, `nil` synth, `cfg.vaultPath`, `cfg.indexMutex`. Increment `toolCount`. (FR-008)
- [x] T023 [US3] Add `dewey curate` CLI command in `cli.go` — implement `newCurateCmd()` with `--store`, `--force`, `--no-embeddings`, `--vault` flags per contracts/cli-commands.md. Create `OllamaSynthesizer` for CLI mode. Run pipeline for each store (or named store). Report results with emoji markers. (FR-008)
- [x] T024 [US3] Add `/dewey-curate` slash command content to `slash_commands.go` — add curate slash command markdown content per contracts/cli-commands.md slash command section. Follow existing slash command pattern.

### Tests

- [x] T025 [P] [US3] Add tests for curation pipeline in `curate/curate_test.go` — test `BuildExtractionPrompt()` includes all document content, test `ParseExtractionResponse()` with valid JSON/malformed JSON/missing fields, test `WriteKnowledgeFile()` creates file with correct frontmatter and body, test `LoadCurationState()`/`SaveCurationState()` round-trip, test `CurateStore()` with `NoopSynthesizer` returning pre-built JSON. Use `t.TempDir()` and in-memory SQLite.
- [x] T026 [P] [US4] Add tests for quality analysis in `curate/curate_test.go` — test quality flags are parsed from LLM response, test confidence scoring validation (high/medium/low/flagged), test `incongruent` flag includes both source references, test `missing_rationale` flag on decisions without explanation.
- [x] T027 [P] [US3] Add tests for curate MCP tool in `tools/curate_test.go` — test tool handler with nil synthesizer returns prompts, test error cases (no store, no config, store not found, indexing in progress), test mutex acquisition prevents concurrent curation.

**Checkpoint**: `dewey curate` extracts knowledge from indexed sources and writes curated markdown files. Quality flags and confidence scores are present. Run `go test -race -count=1 ./curate/... ./tools/... ./store/...` — all tests pass.

---

## Phase 4: Curated Trust Tier (US6)

**Purpose**: Introduce the `curated` trust tier for machine-extracted knowledge. Enable filtering by tier in semantic search.

**Goal**: `semantic_search_filtered(tier: "curated")` returns only knowledge store content.

**Independent Test**: Curate a knowledge file → query with `tier: "curated"` → verify only curated content appears → query with `tier: "authored"` → verify curated content does NOT appear.

### Implementation

- [x] T028 [US6] Set `tier = "curated"` on knowledge store pages in `curate/curate.go` — when writing knowledge files, set `Tier: "curated"` in the `KnowledgeFile` struct. When auto-indexing knowledge store directories, ensure pages from `knowledge-*` sources get `tier: "curated"`. Add tier detection in the indexing path for `knowledge-` prefixed source IDs. (FR-022, FR-023)
- [x] T029 [US6] Verify `curated` tier filtering in `tools/semantic.go` — confirm `filterResultsByTier()` works with `"curated"` string (string equality — should work without code changes). If any code change is needed, add it. Update tier documentation in code comments to reflect the full hierarchy: `authored > curated > validated > draft`. (FR-024)

### Tests

- [x] T030 [P] [US6] Add tests for curated tier filtering in `tools/semantic_test.go` — test `semantic_search_filtered` with `tier: "curated"` returns only curated pages, test `tier: "authored"` excludes curated pages, test `tier: "draft"` excludes curated pages. Use in-memory SQLite with pre-inserted pages at different tiers.

**Checkpoint**: Curated content is filterable by tier. Existing tier behavior unchanged. Run `go test -race -count=1 ./tools/...` — all tests pass.

---

## Phase 5: Continuous Background Curation (US5)

**Purpose**: Background goroutine periodically checks for new indexed content and curates incrementally. Knowledge stores stay current without manual intervention.

**Goal**: New content indexed during `dewey serve` is automatically curated within the configured interval.

**Independent Test**: Start `dewey serve` with a configured knowledge store → add a new markdown file to a mapped source directory → wait for background curation interval → verify a new knowledge file appears in the store.

### Implementation

- [x] T031 [US5] Implement background curation goroutine in `main.go` — add `backgroundCuration()` function following the spec 012 background indexing pattern. Wait for `indexReady` before first run. Create per-store tickers with configurable intervals. Use `TryLock()` on `indexMu` — skip cycle if indexing in progress. Create `OllamaSynthesizer` for background mode (nil if Ollama unavailable — log and skip). Log errors and continue polling (never crash goroutine). (FR-017, FR-020, FR-021)
- [x] T032 [US5] Wire background curation launch in `main.go` `executeServe()` — after background indexing goroutine, load `knowledge-stores.yaml`, launch `backgroundCuration()` if stores are configured and `!noEmbeddings`. Pass `ctx`, `indexMu`, `indexReady`, store configs, persistent store, embedder, vault path. (FR-017, FR-018)
- [x] T033 [US5] Implement configurable polling interval in `curate/config.go` — add `ParsedInterval() time.Duration` method to `StoreConfig` that returns the parsed `CurationInterval` (default 10 minutes). Used by background goroutine to create per-store tickers. (FR-018)

### Tests

- [x] T034 [P] [US5] Add tests for background curation lifecycle in `main_test.go` or `curate/curate_test.go` — test goroutine respects context cancellation, test `TryLock()` skips when mutex is held, test goroutine continues after curation error, test goroutine waits for `indexReady` before first run. Use short intervals (10ms) and mock synthesizer.

**Checkpoint**: Background curation runs automatically during `dewey serve`. Errors are logged without crashing. Mutex prevents concurrent operations. Run `go test -race -count=1 ./...` — all tests pass.

---

## Phase 6: Lint Integration + Auto-Indexing (US7, US8)

**Purpose**: Extend `dewey lint` with knowledge store quality metrics. Auto-register knowledge store directories as disk sources for immediate searchability.

**Goal**: `dewey lint` reports aggregate knowledge quality. Curated files are automatically searchable.

**Independent Test**: Curate knowledge with quality flags → run `dewey lint` → verify report includes knowledge store metrics. Query `semantic_search` for curated content → verify results appear.

### Implementation

- [x] T035 [US7] Extend `tools/lint.go` with knowledge store quality metrics — add `knowledge_quality` and `stale_knowledge` finding types. Scan knowledge store directories for curated files, parse frontmatter to count confidence levels and quality flag types. Detect stale stores (sources updated since last curation checkpoint). Add `knowledge_quality_issues` and `stale_knowledge_stores` to lint summary. (FR-025, FR-026)
- [x] T036 [US8] Implement auto-registration of knowledge store directories as disk sources — after curation writes files, check if `knowledge-{store-name}` source exists in the store's `sources` table. If not, register it as a disk source. Trigger indexing for the new source. Set `tier: "curated"` on auto-indexed pages. Add skip logic in `purgeOrphanedSources()` for `knowledge-` prefixed sources. (FR-027, FR-028)

### Tests

- [x] T037 [P] [US7] Add tests for lint knowledge store checks in `tools/lint_test.go` — test `knowledge_quality` finding with low-confidence facts, test `stale_knowledge` finding with unprocessed documents, test lint summary includes new fields, test lint with no knowledge stores configured produces no new findings.
- [x] T038 [P] [US8] Add tests for auto-indexing in `curate/curate_test.go` or `tools/curate_test.go` — test knowledge store directory is registered as disk source after curation, test `knowledge-{name}` source ID format, test `purgeOrphanedSources()` skips `knowledge-` prefixed sources, test auto-indexed pages have `tier: "curated"`.

**Checkpoint**: `dewey lint` reports knowledge store quality. Curated files are automatically indexed and searchable. Run `go test -race -count=1 ./...` — all tests pass.

---

## Phase 7: Integration + Documentation

**Purpose**: End-to-end validation, documentation updates, and CI parity gate.

**Goal**: The complete feature works end-to-end. Documentation is current. CI passes.

### Integration

- [x] T039 [US3] End-to-end integration test — index a source with test content → configure a knowledge store → run `dewey curate` → verify knowledge files created with correct frontmatter → query `semantic_search` for curated content → verify results appear with `tier: "curated"` → run `dewey lint` → verify knowledge store metrics in report. Use `t.TempDir()`, in-memory SQLite, and `NoopSynthesizer`.

### Documentation

- [x] T040 [P] Update `AGENTS.md` — add `curate/` package to Architecture section, add `dewey curate` to CLI Commands table, add `knowledge-stores.yaml` to Content Source Types or new Knowledge Stores section, update Active Technologies with new dependencies, document `curated` trust tier in Trust Tiers table, add knowledge store configuration to Coding Conventions if needed.
- [x] T041 [P] Update `README.md` — add Knowledge Stores section describing the feature, add `dewey curate` to command list, add `knowledge-stores.yaml` configuration example, document the `curated` trust tier.
- [x] T042 [P] Create website documentation sync issue — run `gh issue create --repo unbound-force/website` with title `docs: sync dewey curated knowledge stores documentation` describing new `dewey curate` command, `knowledge-stores.yaml` config, `curated` trust tier, and affected website pages.

### CI Parity Gate

- [x] T043 Run CI parity gate — read `.github/workflows/ci.yml` and `.github/workflows/mega-linter.yml` to identify exact CI commands. Execute locally: `go build ./...`, `go vet ./...`, `go test -race -count=1 ./...`, `gaze report ./... --coverprofile=coverage.out --max-crapload=48 --max-gaze-crapload=18 --min-contract-coverage=70`. All must pass. Fix any failures before declaring implementation complete.

**Checkpoint**: All tests pass. Documentation is updated. CI parity gate passes. Feature is complete.

---

## Dependencies & Execution Order

### Phase Dependencies

- **Phase 1 (File-Backed Learnings)**: No dependencies — start immediately. Foundation for all other phases.
- **Phase 2 (Configuration)**: No strict dependency on Phase 1, but logically follows. Can start after Phase 1 or in parallel if different developers.
- **Phase 3 (Curation Pipeline)**: Depends on Phase 2 (config parsing). Core intelligence — largest phase.
- **Phase 4 (Curated Tier)**: Depends on Phase 3 (curated pages must exist to filter).
- **Phase 5 (Background Curation)**: Depends on Phase 3 (pipeline must exist to run in background).
- **Phase 6 (Lint + Auto-Index)**: Depends on Phase 3 (knowledge files must exist to lint/index).
- **Phase 7 (Integration + Docs)**: Depends on all prior phases.

### Within Each Phase

- Tasks without `[P]` are sequential — complete each before starting the next
- Tasks with `[P]` can run in parallel (different files, no dependencies)
- Tests marked `[P]` can run in parallel with each other
- Implementation tasks must complete before their corresponding test tasks

### Parallel Opportunities

```
Phase 1: T001 → T002 → T003 → [T004, T005] (parallel tests)
Phase 2: T006 → T007 → T008 → [T009] (tests)
Phase 3: T010 → T011 → T012 → T013 → T014 → T015 → T016 → T017 → T018 → T019 → T020 → T021 → T022 → T023 → T024 → [T025, T026, T027] (parallel tests)
Phase 4: T028 → T029 → [T030] (tests)
Phase 5: T031 → T032 → T033 → [T034] (tests)
Phase 6: [T035, T036] (parallel — different files) → [T037, T038] (parallel tests)
Phase 7: T039 → [T040, T041, T042] (parallel docs) → T043
```

### Story-to-Task Mapping

| Story | Tasks | Phase |
|-------|-------|-------|
| US1 — File-Backed Learning Persistence | T001–T005 | Phase 1 |
| US2 — Knowledge Store Configuration | T006–T009 | Phase 2 |
| US3 — Source-Mapped Knowledge Extraction | T010–T013, T016–T027 | Phase 3 |
| US4 — Quality Analysis During Curation | T014–T015, T026 | Phase 3 |
| US5 — Continuous Background Curation | T031–T034 | Phase 5 |
| US6 — Curated Trust Tier | T028–T030 | Phase 4 |
| US7 — Knowledge Store Lint Integration | T035, T037 | Phase 6 |
| US8 — Auto-Indexed Knowledge Stores | T036, T038 | Phase 6 |

### FR-to-Task Traceability

| FR | Task(s) | Description |
|----|---------|-------------|
| FR-001 | T001, T002 | Dual-write to SQLite + markdown |
| FR-002 | T002 | YAML frontmatter format |
| FR-003 | T003 | Re-ingestion on startup |
| FR-004 | T003 | Preserve original metadata on re-ingestion |
| FR-005 | T006 | knowledge-stores.yaml support |
| FR-006 | T006, T007 | Store config fields + validation |
| FR-007 | T008 | dewey init scaffolding |
| FR-008 | T010, T019, T020, T023 | Curate CLI + MCP tool |
| FR-009 | T013, T016 | LLM extraction + response parsing |
| FR-010 | T013, T017 | Source traceability |
| FR-011 | T017 | Temporal resolution |
| FR-012 | T017 | Markdown with YAML frontmatter output |
| FR-013 | T014 | Missing information detection |
| FR-014 | T014 | Implied information detection |
| FR-015 | T014 | Incongruent information detection |
| FR-016 | T015 | Confidence scoring |
| FR-017 | T031, T032 | Background curation goroutine |
| FR-018 | T032, T033 | Configurable polling interval |
| FR-019 | T012, T018, T019 | Incremental curation |
| FR-020 | T031 | Shared mutex with index/reindex |
| FR-021 | T031 | Error resilience in background goroutine |
| FR-022 | T028 | Curated tier support |
| FR-023 | T028 | Default tier for curated content |
| FR-024 | T029 | semantic_search_filtered tier filtering |
| FR-025 | T035 | Lint knowledge quality metrics |
| FR-026 | T035 | Lint stale knowledge detection |
| FR-027 | T036 | Auto-register knowledge store directories |
| FR-028 | T036 | knowledge-{store-name} source ID format |

---

## Implementation Strategy

### MVP First (Phase 1 + Phase 2 + Phase 3)

1. Complete Phase 1: File-Backed Learnings (US1) — foundation
2. Complete Phase 2: Configuration (US2) — contract
3. Complete Phase 3: Curation Pipeline (US3 + US4) — core value
4. **STOP and VALIDATE**: Test curation end-to-end manually
5. This delivers the core feature: file-backed learnings + knowledge extraction

### Incremental Delivery

1. Phase 1 → Learnings are durable (immediate value)
2. Phase 2 → Configuration ready (no user-visible change yet)
3. Phase 3 → `dewey curate` works (core feature delivered)
4. Phase 4 → Curated content is filterable by tier
5. Phase 5 → Background curation keeps stores current
6. Phase 6 → Lint reports quality, stores are auto-searchable
7. Phase 7 → Documentation and CI validation

---

## Notes

- [P] tasks = different files, no dependencies
- [Story] label maps task to specific user story for traceability
- All 28 FRs are covered by at least one task (see FR-to-Task traceability)
- The `curated` tier requires no schema migration — it's a new value in the existing `tier TEXT` column
- Background curation uses the established goroutine + mutex pattern from specs 011/012
- LLM-dependent tests use `llm.NoopSynthesizer` — no real Ollama needed
- File system tests use `t.TempDir()` — no external dependencies
- Store tests use in-memory SQLite (`:memory:`) — no disk I/O
<!-- spec-review: passed -->
