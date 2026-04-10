# Tasks: Knowledge Compilation & Temporal Intelligence

**Input**: Design documents from `/specs/013-knowledge-compile/`
**Prerequisites**: plan.md (required), spec.md (required), research.md, quickstart.md, contracts/ (6 contracts)

**Spec**: 6 user stories, 28 functional requirements, 6 contracts
**Branch**: `013-knowledge-compile`

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (e.g., US1, US2)
- Include exact file paths in descriptions

---

## Phase 1: Schema Migration + Store Layer (Foundation)

**Purpose**: Add `tier` and `category` columns to the pages table, update all page CRUD operations, and add new store helpers. This is the data layer foundation that all user stories depend on.

**⚠️ CRITICAL**: No user story work can begin until this phase is complete.

- [x] T001 [US1,US5] Update schema version constant and `createSchema()` in `store/migrate.go` — add `tier TEXT DEFAULT 'authored'` and `category TEXT` columns to the `pages` CREATE TABLE statement, add `CREATE INDEX idx_pages_tier ON pages(tier)`, update `schemaVersion` to `2` (contracts/schema-migration.md)
- [x] T002 [US1,US5] Implement `migrateV1toV2()` in `store/migrate.go` — `ALTER TABLE pages ADD COLUMN tier/category`, backfill `tier='draft'` for `source_id='learning'` and `source_id='compiled'`, create `idx_pages_tier` index, update `schema_version` metadata to `2`. Must be idempotent and run within a single transaction (contracts/schema-migration.md)
- [x] T003 [US1,US5] Update `Page` struct in `store/store.go` — add `Tier string` and `Category string` fields. Update `InsertPage`, `GetPage`, `ListPages`, `UpdatePage`, `ListPagesExcludingSource`, `ListPagesBySource` to include `tier` and `category` in SQL INSERT/SELECT/UPDATE statements (contracts/schema-migration.md)
- [x] T004 [US1] Add store helper `NextLearningSequence(tag string) (int, error)` in `store/store.go` — counts existing learning pages with matching tag prefix (`name LIKE 'learning/' || ? || '-%'`) and returns count+1 (contracts/store-learning.md)
- [x] T005 [US4] Add store helper `ListLearningPages() ([]*Page, error)` in `store/store.go` — returns all pages with `source_id = 'learning'` ordered by name (contracts/lint-tool.md)
- [x] T006 [P] [US4] Add store helper `PagesWithoutEmbeddings() ([]*Page, error)` in `store/store.go` — LEFT JOIN pages→blocks→embeddings, returns pages with blocks but no embeddings (contracts/lint-tool.md)
- [x] T007 [P] [US5] Add store helper `UpdatePageTier(name, tier string) error` in `store/store.go` — `UPDATE pages SET tier = ?, updated_at = ? WHERE name = ?`, returns error if page not found (contracts/promote-tool.md)
- [x] T008 [US1,US5] Write tests for schema migration and new store helpers in `store/migrate_test.go` — test v1→v2 migration (columns added, defaults applied, learning pages get `tier='draft'`, idempotency), test `NextLearningSequence`, test `UpdatePageTier`, test `ListLearningPages`, test `PagesWithoutEmbeddings`

**Checkpoint**: Schema v2 is in place. All page CRUD operations handle `tier` and `category`. Store helpers are tested. Foundation ready for user story implementation.

---

## Phase 2: Learning API Update (US1 — Temporal Awareness)

**Purpose**: Update `store_learning` MCP tool with required `tag` parameter, `{tag}-{sequence}` identity, `category` support, and `tier=draft`. This is the P1 core that compilation depends on.

**Goal**: An agent can store a learning with a topic tag and receive a human-readable `{tag}-{sequence}` identity with temporal metadata.

**Independent Test**: Store two learnings with the same tag. Verify both have `{tag}-{sequence}` identities, `created_at` timestamps, and `tier=draft`.

- [x] T009 [US1] Update `StoreLearningInput` in `types/tools.go` — replace `Tags string` with `Tag string` (required, topic namespace) and add `Category string` (optional enum: decision/pattern/gotcha/context/reference). Keep deprecated `Tags string` with `json:"tags,omitempty"` for backward compatibility (contracts/store-learning.md, FR-001)
- [x] T010 [US1] Update `StoreLearning` handler in `tools/learning.go` — implement tag resolution (tag > tags > "general"), tag normalization (lowercase, trim, hyphens), `{tag}-{sequence}` identity via `NextLearningSequence`, page naming as `learning/{tag}-{sequence}`, set `Tier="draft"`, set `Category` from input, add `created_at` ISO 8601 and `tag` to properties JSON, return identity/page/tag/category/created_at in response (contracts/store-learning.md, FR-001 through FR-005)
- [x] T011 [US1] Add category validation in `tools/learning.go` — validate `category` against allowed values (decision/pattern/gotcha/context/reference), return MCP error for invalid category, allow empty category (contracts/store-learning.md)
- [x] T012 [US1] Update tests in `tools/learning_test.go` — test new `tag` parameter with `{tag}-{sequence}` identity, test category validation (valid/invalid/empty), test backward compatibility (`tags` field fallback), test tag normalization, test default tag "general", test `created_at` in response, test `tier=draft` on stored page (contracts/store-learning.md, FR-001 through FR-005)

**Checkpoint**: `store_learning` accepts `tag` and `category`, returns `{tag}-{sequence}` identity. Backward compatible with old `tags` parameter. All learnings stored with `tier=draft` and `created_at`.

---

## Phase 3: Search Metadata Enrichment (US1, US5)

**Purpose**: Add `created_at`, `category`, and `tier` to semantic search results. Add `tier` filter to `semantic_search_filtered`. Agents can now see temporal metadata and filter by trust tier.

**Goal**: Search results include provenance metadata. Agents can filter by trust tier for high-confidence context.

**Independent Test**: Store a learning (tier: draft). Search with tier filter "authored". Verify learning does not appear. Search without filter. Verify it appears with `created_at` and `tier` metadata.

- [x] T013 [P] [US1,US5] Update `SemanticSearchResult` in `types/tools.go` — add `CreatedAt string`, `Tier string`, and `Category string` fields to the result type (research.md R10, FR-004)
- [x] T014 [P] [US5] Update `SemanticSearchFilteredInput` in `types/tools.go` — add `Tier string` field with `json:"tier,omitempty"` for tier-based filtering (FR-024)
- [x] T015 [US5] Update `SearchFilters` in `store/embeddings.go` — add `Tier string` field. When non-empty, append `AND p.tier = ?` to the filtered similarity query SQL (FR-024, quickstart.md D9). **Implementation note**: Tier filtering implemented as post-query filter in `tools/semantic.go` instead of modifying `store/embeddings.go`, keeping the change scoped to the tools layer.
- [x] T016 [US1,US5] Update `toSemanticResults()` in `tools/semantic.go` — populate `CreatedAt` (convert Unix ms → ISO 8601), `Tier`, and `Category` from page data in result metadata (FR-004, research.md R10)
- [x] T017 [US1,US5] Update `tools/semantic_test.go` — test that semantic search results include `created_at`, `tier`, `category` metadata; test tier filtering in `semantic_search_filtered` (returns only matching tier); test that non-learning pages have empty category

**Checkpoint**: Semantic search returns temporal and trust metadata. Tier filtering works. Agents can distinguish authored from draft content.

---

## Phase 4: LLM Synthesis Interface (US2 Foundation)

**Purpose**: Create the `llm/` package with `Synthesizer` interface and Ollama implementation. This is the generation backend for the compile tool.

**Goal**: A pluggable LLM synthesis interface that the compile tool can use for article generation, with a noop implementation for testing.

**Independent Test**: Create an `OllamaSynthesizer` with an `httptest` mock server. Call `Synthesize`. Verify the correct Ollama `/api/generate` request is sent and the response is parsed.

- [x] T018 [P] [US2] Create `llm/llm.go` — define `Synthesizer` interface (`Synthesize(ctx, prompt) (string, error)`, `Available() bool`, `ModelID() string`), implement `OllamaSynthesizer` struct (baseURL, model, http.Client, availability cache with 30s interval), implement `NewOllamaSynthesizer(baseURL, model)`, implement `Synthesize` via POST `/api/generate` with `stream=false` and 120s timeout, implement `Available` with cached health check, implement `ModelID`. Also implement exported `NoopSynthesizer` test double. Package MUST NOT import any Dewey packages (contracts/llm-interface.md)
- [x] T019 [P] [US2] Create `llm/llm_test.go` — test `OllamaSynthesizer.Synthesize` with `httptest` mock (success, error status, malformed response, context cancellation), test `Available` caching behavior, test `NoopSynthesizer` returns configured response/error (contracts/llm-interface.md)

**Checkpoint**: `llm/` package is complete and tested. `Synthesizer` interface is ready for injection into the compile tool.

---

## Phase 5: Compile Tool (US2, US6)

**Purpose**: Implement the core compilation pipeline — clustering learnings by topic, generating synthesis prompts with category-aware instructions, producing compiled articles, and generating the `_index.md`.

**Goal**: `dewey compile` reads all learnings, clusters by tag + semantic similarity, and produces compiled articles in `.uf/dewey/compiled/` with current-state and history sections.

**Independent Test**: Store 5 learnings on the same topic (including one contradiction). Run compile. Verify a compiled article exists that resolves the contradiction in favor of the newer learning.

- [x] T020 [US2] Create `tools/compile.go` — define `Compile` struct with injected dependencies (`store.Store`, `embed.Embedder`, `llm.Synthesizer`, `vaultPath`), implement `NewCompile` constructor, define `LearningEntry` and `Cluster` types, define `CompileInput` in `types/tools.go` with `Incremental []string` field (contracts/compile-tool.md)
- [x] T021 [US2] Implement `clusterLearnings()` pure function in `tools/compile.go` — group by tag, semantic similarity refinement within tag groups (split if cosine < 0.3), cross-tag merge (merge if average similarity > 0.8), topic naming from dominant tag. Must be deterministic (contracts/compile-tool.md, FR-007)
- [x] T022 [US2,US6] Implement category-aware synthesis prompt generation in `tools/compile.go` — build prompts with category-specific instructions: `decision` → temporal merge (newer wins, carry forward non-contradicted), `pattern` → accumulate, `gotcha` → de-duplicate, `context` → carry forward, `reference` → preserve as-is. Include chronological learning list in prompt (FR-008, FR-025, FR-026, FR-027)
- [x] T023 [US2] Implement `Compile` handler method in `tools/compile.go` — full rebuild mode (delete `.uf/dewey/compiled/`, rebuild from all learnings) and incremental mode (merge specified learnings into existing articles). When synthesizer available: call LLM, write articles, index in store with `source_id="compiled"` and `tier="draft"`. When synthesizer nil: return clusters + prompts as structured output. Handle empty learnings case (empty `_index.md` with "No learnings to compile") (contracts/compile-tool.md, FR-006, FR-009, FR-012)
- [x] T024 [US2] Implement compiled article writer in `tools/compile.go` — write markdown files to `.uf/dewey/compiled/` with frontmatter (tier, compiled_at, sources, topic), current-state section, history table. Generate `_index.md` listing all articles with links (FR-009, FR-010, contracts/compile-tool.md)
- [x] T025 [US2,US6] Create `tools/compile_test.go` — test `clusterLearnings` (same tag grouping, cross-tag merge, single learning cluster), test synthesis prompt generation (category-aware instructions), test full rebuild (deletes old, writes new), test incremental mode, test empty learnings case, test with nil synthesizer (prompt-only mode), test compiled article format (frontmatter, current-state, history), test `_index.md` generation, test knowledge evolution (partial supersession per US6 FR-025/FR-026/FR-027)

**Checkpoint**: Compile tool clusters learnings, generates category-aware prompts, produces compiled articles with current-state + history sections. Works in both synthesizer and prompt-only modes.

---

## Phase 6: Lint + Promote Tools (US4, US5)

**Purpose**: Implement knowledge quality linting (4 checks + auto-fix) and trust tier promotion. These are the governance tools for knowledge base health.

**Goal**: `dewey lint` detects stale decisions, uncompiled learnings, embedding gaps, and contradictions. `dewey promote` transitions pages from draft to validated.

**Independent Test (Lint)**: Store a decision learning backdated >30 days. Run lint. Verify it reports as stale.
**Independent Test (Promote)**: Store a learning (tier: draft). Promote it. Verify tier is now validated.

- [x] T026 [US4] Create `tools/lint.go` — define `Lint` struct with injected dependencies (`store.Store`, `embed.Embedder`), implement `NewLint` constructor, define `Finding` struct (type, severity, identity/identities, page, similarity, description, remediation) (contracts/lint-tool.md)
- [x] T027 [US4] Implement 4 lint checks in `tools/lint.go` — `checkStaleDecisions()` (category=decision, >30 days, not validated), `checkUncompiledLearnings()` (learnings not in any compiled article's sources), `checkEmbeddingGaps()` (pages with blocks but no embeddings), `checkContradictions()` (learning pairs with cosine similarity >0.8 and same tag) (FR-016, FR-017, contracts/lint-tool.md)
- [x] T028 [US4] Implement `Lint` handler method and `fixEmbeddingGaps()` in `tools/lint.go` — aggregate findings from all 4 checks, when `fix=true` regenerate embeddings for gap pages via embedder, return structured report with summary counts and actionable remediation per finding (FR-018, FR-019, contracts/lint-tool.md)
- [x] T029 [US5] Create `tools/promote.go` — define `Promote` struct with injected `store.Store`, implement `NewPromote` constructor, implement `Promote` handler: validate page exists, validate current tier is `draft`, call `UpdatePageTier(name, "validated")`, return success with previous/new tier. Error cases: page not found, page not draft, store unavailable (contracts/promote-tool.md, FR-023)
- [x] T030 [P] [US4] Create `tools/lint_test.go` — test each lint check independently with fixture data (stale decisions, uncompiled learnings, embedding gaps, contradictions), test `--fix` repairs embedding gaps, test clean report when no issues, test lint without embedder (skip contradiction check), test lint without store (error) (contracts/lint-tool.md)
- [x] T031 [P] [US5] Create `tools/promote_test.go` — test draft→validated promotion, test rejection of authored page, test rejection of already-validated page, test page not found, test store unavailable, test `updated_at` is refreshed on promotion (contracts/promote-tool.md)

**Checkpoint**: Lint detects all 4 quality issue types, auto-fixes embedding gaps. Promote transitions draft→validated. Both tools return structured, actionable results.

---

## Phase 7: Server Registration + CLI Commands (All Stories)

**Purpose**: Wire the 3 new MCP tools into the server and add 3 new CLI commands. This makes all new capabilities accessible via both MCP and command line.

- [x] T032 [US2,US4,US5] Register `compile`, `lint`, and `promote` tools in `server.go` — follow existing `mcp.AddTool` pattern, inject store/embedder/synthesizer dependencies, use tool descriptions from contracts (contracts/compile-tool.md, contracts/lint-tool.md, contracts/promote-tool.md)
- [x] T033 [US2,US4,US5] Add `dewey compile` CLI command in `cli.go` — `newCompileCmd()` with `--incremental/-i` flag (repeatable), opens store, creates OllamaSynthesizer from config, runs compilation, prints results (contracts/compile-tool.md)
- [x] T034 [US4] Add `dewey lint` CLI command in `cli.go` — `newLintCmd()` with `--fix` flag, opens store, creates embedder, runs lint, prints structured report, exit code 0 if clean / 1 if issues (contracts/lint-tool.md)
- [x] T035 [US5] Add `dewey promote` CLI command in `cli.go` — `newPromoteCmd()` with positional `PAGE_NAME` argument, opens store, runs promote, prints result (contracts/promote-tool.md)
- [x] T036 [US2,US4,US5] Update `server_test.go` — verify tool count increased by 3 (compile, lint, promote). Update any existing tool registration assertions.

**Checkpoint**: All 3 new tools are accessible via MCP and CLI. Server test validates correct tool count.

---

## Phase 8: Session-End Integration (US3)

**Purpose**: Integrate incremental compilation into the `/unleash` retrospective flow. Compilation runs automatically after learnings are stored, keeping the knowledge base current.

**Goal**: After every session's retrospective stores learnings, incremental compilation merges them into existing compiled articles automatically.

**Independent Test**: Verify the `/unleash` command file includes the compile trigger with non-blocking error handling.

- [x] T037 [US3] Update `.opencode/command/unleash.md` — add compile trigger after the retrospective stores learnings. Wrap in error handling so compilation failure is non-blocking (FR-013, FR-014, FR-015). This is a documentation/instruction change, not a code change.

**Checkpoint**: Session-end auto-compile is configured. Compilation failure does not block retrospective completion.

---

## Phase 9: Integration Testing + Documentation

**Purpose**: End-to-end validation and documentation updates.

- [x] T038 [US2,US6] Write integration test — end-to-end flow: store 3+ learnings with same tag (including temporal contradiction) → run compile → verify compiled article exists with correct current-state and history → search for compiled article via semantic search → verify it appears with `tier=draft` metadata (SC-002, SC-007)
- [x] T039 Update `AGENTS.md` — add `llm/` package to Architecture section, add `dewey compile`, `dewey lint`, `dewey promote` to CLI Commands table, update tool count (40 → 43), add schema v2 to Active Technologies, document `compile` content source type alongside existing source types, add `tier` and `category` to key patterns

**Checkpoint**: End-to-end flow validated. Documentation reflects all new capabilities.

---

## Phase 10: Verification

**Purpose**: CI parity gate — ensure all checks pass before declaring implementation complete.

- [x] T040 Run CI parity gate — read `.github/workflows/ci.yml` to identify exact commands, then execute: `go build ./...`, `go vet ./...`, `go test -race -count=1 ./...`, `go test -race -count=1 -coverprofile=coverage.out ./...`, `gaze report ./... --coverprofile=coverage.out --max-crapload=48 --max-gaze-crapload=18 --min-contract-coverage=70`. All must pass. Fix any failures before declaring complete.

**Checkpoint**: All CI checks pass locally. Implementation is complete and ready for review council.

---

## Dependencies & Execution Order

### Phase Dependencies

- **Phase 1 (Schema + Store)**: No dependencies — start immediately. BLOCKS all subsequent phases.
- **Phase 2 (Learning API)**: Depends on Phase 1 (needs `tier`, `category` columns and `NextLearningSequence`)
- **Phase 3 (Search Metadata)**: Depends on Phase 1 (needs `tier` column in queries). Can run in parallel with Phase 2.
- **Phase 4 (LLM Interface)**: No dependencies on other phases — can run in parallel with Phases 2 and 3.
- **Phase 5 (Compile)**: Depends on Phases 2 (learning identity format), 3 (search metadata), and 4 (synthesizer interface)
- **Phase 6 (Lint + Promote)**: Depends on Phase 1 (store helpers). Can start after Phase 1, in parallel with Phases 2-4.
- **Phase 7 (Server + CLI)**: Depends on Phases 5 and 6 (tools must exist before registration)
- **Phase 8 (Session-End)**: Depends on Phase 7 (compile tool must be registered)
- **Phase 9 (Integration + Docs)**: Depends on Phases 7 and 8
- **Phase 10 (Verification)**: Depends on all previous phases

### Parallel Opportunities

```
Phase 1 (Schema + Store)
    │
    ├──→ Phase 2 (Learning API)  ──┐
    ├──→ Phase 3 (Search Metadata) ├──→ Phase 5 (Compile) ──┐
    ├──→ Phase 4 (LLM Interface) ──┘                        ├──→ Phase 7 (Server + CLI)
    └──→ Phase 6 (Lint + Promote) ──────────────────────────┘        │
                                                                      ├──→ Phase 8 (Session-End)
                                                                      └──→ Phase 9 (Integration + Docs)
                                                                               │
                                                                               └──→ Phase 10 (Verification)
```

### Within Each Phase

- Tasks marked [P] can run in parallel (different files, no dependencies)
- Tasks without [P] must be completed sequentially in listed order
- Tests should be written alongside or immediately after the code they test

### User Story Traceability

| Story | Priority | Tasks | Key FRs |
|-------|----------|-------|---------|
| US1 — Temporal Awareness | P1 | T001-T004, T008-T012, T013, T016-T017 | FR-001 through FR-005 |
| US2 — Knowledge Compilation | P1 | T018-T025, T032-T033, T036, T038 | FR-006 through FR-012 |
| US3 — Session-End Auto-Compile | P1 | T037 | FR-013 through FR-015 |
| US4 — Knowledge Linting | P2 | T005-T006, T026-T028, T030, T034, T036 | FR-016 through FR-019 |
| US5 — Contamination Separation | P2 | T007, T013-T015, T017, T029, T031, T035-T036 | FR-020 through FR-024 |
| US6 — Knowledge Evolution | P3 | T022, T025, T038 | FR-025 through FR-028 |

---

## Notes

- [P] tasks = different files, no dependencies
- [Story] label maps task to specific user story for traceability
- Each phase has a checkpoint — validate before proceeding
- The `llm/` package (Phase 4) is a leaf dependency — it MUST NOT import any Dewey packages
- Compiled articles are ephemeral — full rebuild replaces the entire `.uf/dewey/compiled/` directory
- The `/unleash` integration (Phase 8) is a documentation change, not a code change
- Schema migration must be idempotent — running v1→v2 twice must not error
- Backward compatibility: old `tags` parameter must still work via fallback logic
<!-- spec-review: passed -->
