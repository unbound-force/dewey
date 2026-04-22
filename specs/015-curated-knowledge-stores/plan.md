# Implementation Plan: Curated Knowledge Stores

**Branch**: `015-curated-knowledge-stores` | **Date**: 2026-04-21 | **Spec**: [spec.md](spec.md)
**Input**: Feature specification from `specs/015-curated-knowledge-stores/spec.md`

## Summary

Curated Knowledge Stores introduces three layers of knowledge management to Dewey: file-backed learning persistence (dual-write to SQLite + markdown with startup re-ingestion), a configurable curation pipeline that uses LLMs to extract structured knowledge from indexed sources with quality analysis and confidence scoring, and continuous background curation. This is Dewey's largest spec to date (8 user stories, 28 functional requirements) and touches every layer of the architecture — from the store schema to MCP tools to CLI commands to background goroutines.

**Technical approach**: Extend the existing `tools/learning.go` for file-backed persistence, create a new `curate/` package for the curation pipeline and configuration, add a `curated` trust tier value to the existing tier column, extend `tools/lint.go` for knowledge store quality metrics, and wire background curation into `main.go` using the established goroutine + mutex pattern from specs 011/012.

## Technical Context

**Language/Version**: Go 1.25 (per `go.mod`)
**Primary Dependencies**: `modernc.org/sqlite` (pure-Go SQLite), `github.com/modelcontextprotocol/go-sdk` (MCP SDK), `github.com/spf13/cobra` (CLI), `github.com/charmbracelet/log` (logging), `gopkg.in/yaml.v3` (config parsing), `github.com/unbound-force/dewey/llm` (LLM synthesis interface)
**Storage**: SQLite via `modernc.org/sqlite` — single database `.uf/dewey/graph.db`. Schema v2 (pages with tier/category columns). No schema migration needed — `curated` is a new value in the existing `tier TEXT` column.
**Testing**: Standard library `testing` package only. In-memory SQLite (`:memory:`) for store tests. `llm.NoopSynthesizer` for LLM-dependent tests. `t.TempDir()` for filesystem tests.
**Target Platform**: macOS/Linux CLI + MCP server (stdio/HTTP transport)
**Project Type**: CLI tool + MCP server
**Performance Goals**: Background curation must not block MCP tool responses. Incremental curation processes only new/changed documents.
**Constraints**: No CGO. All data stays local (Ollama for LLM). Background curation shares the existing `indexMu` mutex. File-backed learnings must survive `graph.db` deletion.
**Scale/Scope**: 8 user stories, 28 FRs. New `curate/` package (~4 files). Modifications to 8+ existing files.

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

### I. Composability First — **PASS**

- Dewey remains independently installable. The curation pipeline requires Ollama (same as `dewey compile` from spec 013) but degrades gracefully when unavailable — `dewey serve` starts normally, curation is skipped with a log message.
- Knowledge store configuration (`knowledge-stores.yaml`) is optional — Dewey works identically without it.
- File-backed learnings are plain markdown files — readable by any tool, not just Dewey.
- The `curated` tier is a new value in an existing column — no schema migration, no breaking changes.

### II. Autonomous Collaboration — **PASS**

- The `curate` MCP tool and CLI command communicate via structured JSON responses (same pattern as `compile`, `lint`, `promote`).
- Background curation is internal to the server process — it does not require external coordination.
- Knowledge store files are markdown with YAML frontmatter — the standard artifact format for agent communication.
- No runtime coupling, shared memory, or direct function calls with other tools.

### III. Observable Quality — **PASS**

- Every curated knowledge file includes source traceability (source ID, document name, block reference, excerpt) in YAML frontmatter.
- Quality flags (missing_rationale, implied_assumption, incongruent) and confidence scores (high/medium/low/flagged) are stored in frontmatter — auditable at rest.
- `dewey lint` reports aggregate knowledge store quality metrics.
- `dewey status` and `health` tool continue to report index state including curated content.
- The `curated` tier enables filtering in `semantic_search_filtered` — agents can control result quality.

### IV. Testability — **PASS**

- New `curate/` package is testable in isolation with in-memory SQLite and `llm.NoopSynthesizer`.
- File-backed learning tests use `t.TempDir()` — no external dependencies.
- Background curation tests use short intervals and mock synthesizers — no real Ollama needed.
- All existing tests continue to pass — the `curated` tier is additive, not breaking.
- Coverage strategy: Contract tests for the curation pipeline (input → output), unit tests for config parsing, integration tests for file-backed persistence + re-ingestion.

## Project Structure

### Documentation (this feature)

```text
specs/015-curated-knowledge-stores/
├── plan.md              # This file
├── research.md          # Phase 0 output
├── quickstart.md        # Phase 1 output
├── contracts/           # Phase 1 output
│   ├── curate-package.md
│   ├── mcp-tools.md
│   └── cli-commands.md
└── tasks.md             # Phase 2 output (/speckit.tasks)
```

### Source Code (repository root)

```text
# New package
curate/
├── curate.go            # Curation pipeline: extract, analyze, write knowledge files
├── curate_test.go       # Contract tests for curation pipeline
├── config.go            # knowledge-stores.yaml parsing and validation
└── config_test.go       # Config parsing tests

# Modified files
tools/
├── learning.go          # Add file-backed dual-write (FR-001, FR-002)
├── learning_test.go     # Tests for file-backed persistence
├── lint.go              # Add knowledge store quality metrics (FR-025, FR-026)
├── lint_test.go         # Tests for new lint checks
├── semantic.go          # Verify curated tier filtering works (FR-024)
├── curate.go            # MCP tool handler for curate (FR-008)
└── curate_test.go       # MCP tool tests

store/
├── store.go             # Add curation checkpoint queries
└── store_test.go        # Tests for new store methods

main.go                  # Background curation goroutine (FR-017), learning re-ingestion (FR-003)
server.go                # Register curate MCP tool
cli.go                   # dewey curate command, dewey init knowledge-stores.yaml scaffold
slash_commands.go         # Add dewey-curate.md slash command content
```

**Structure Decision**: The `curate/` package follows the same pattern as `llm/` — a leaf dependency with a clear interface. The curation pipeline logic lives in `curate/curate.go` (pure business logic), while the MCP tool handler in `tools/curate.go` is a thin adapter (same pattern as `tools/compile.go` wrapping the compile logic). Config parsing lives in `curate/config.go` following the `source/config.go` pattern.

## Complexity Tracking

> No constitution violations. All complexity is justified by the spec's 28 FRs.

| Design Choice | Justification | Simpler Alternative Rejected Because |
|---------------|---------------|--------------------------------------|
| New `curate/` package vs inline in `tools/` | Curation pipeline has config parsing, LLM interaction, file I/O, and quality analysis — too much for a single tool handler. Separation of concerns (SRP). | Inline in `tools/curate.go` would create a 500+ line file mixing config, pipeline, and tool handler concerns. |
| File-backed learnings in `tools/learning.go` vs new package | The dual-write is a small addition to the existing `StoreLearning` function (~30 lines). A new package would over-abstract. | N/A — keeping it in `tools/learning.go` is the simpler choice. |
| Background curation in `main.go` vs `curate/` package | Background goroutine lifecycle management (mutex, context, shutdown) belongs in the orchestration layer (`main.go`), not the business logic layer (`curate/`). Same pattern as background indexing (spec 012). | Putting it in `curate/` would require passing `sync.Mutex`, `context.Context`, and shutdown channels — leaking orchestration concerns into business logic. |

---

## Phase 0: Research

### R1: File-Backed Learning Persistence Pattern

**Question**: How should the dual-write to markdown files work in `tools/learning.go`?

**Finding**: The current `StoreLearning` function (lines 98-206 of `tools/learning.go`) inserts a page into the store, parses content into blocks, persists blocks, and generates embeddings. The dual-write should happen after the store insert succeeds but before the response is returned. The markdown file format should match the existing learning page structure:

```yaml
---
tag: authentication
category: decision
created_at: "2026-04-21T10:30:00Z"
identity: authentication-3
tier: draft
---
```

Followed by the learning content as markdown body text.

**File path**: `.uf/dewey/learnings/{tag}-{seq}.md` (e.g., `.uf/dewey/learnings/authentication-3.md`).

**Re-ingestion on startup**: In `main.go`, after the store is opened but before background indexing starts, scan `.uf/dewey/learnings/` for `.md` files. For each file, parse the YAML frontmatter to extract `identity`. Check if a page with `name = "learning/{identity}"` exists in the store. If not, re-ingest by calling the same store insert + block persist + embedding generation pipeline.

**Risk**: The `Learning` struct currently doesn't have access to the vault path (it only has `embedder` and `store`). We need to either:
- (a) Add a `vaultPath` field to `Learning` (matches `Compile` pattern), or
- (b) Pass the learnings directory path directly.

**Decision**: Option (a) — add `vaultPath string` to `Learning` struct. This matches the `Compile` struct pattern and provides flexibility for future file operations.

### R2: Knowledge Store Configuration Schema

**Question**: What should `knowledge-stores.yaml` look like?

**Finding**: Following the `sources.yaml` pattern from `source/config.go`:

```yaml
# Knowledge store configuration
# Each store curates knowledge from indexed sources.

stores:
  - name: team-decisions
    sources: [disk-meetings, disk-slack-export]
    path: .uf/dewey/knowledge/team-decisions
    curate_on_index: true
    curation_interval: 10m
```

**Fields**:
- `name` (required): Unique store identifier
- `sources` (required): List of source IDs from `sources.yaml`
- `path` (optional): Output directory for curated files. Default: `.uf/dewey/knowledge/{name}`
- `curate_on_index` (optional): Auto-curate after `dewey index`. Default: `false`
- `curation_interval` (optional): Background polling interval. Default: `10m`

**Parsing**: New `curate/config.go` with `LoadKnowledgeStoresConfig(path string) ([]StoreConfig, error)` following the `source.LoadSourcesConfig` pattern.

### R3: Curation Pipeline Architecture

**Question**: How does the curation pipeline extract knowledge from indexed content?

**Finding**: The pipeline follows the `tools/compile.go` pattern but operates on source content rather than learnings:

1. **Read indexed content**: Query store for pages belonging to the configured source IDs. Filter to pages updated since the last curation checkpoint.
2. **Build context**: For each page, retrieve blocks and concatenate into a document.
3. **LLM extraction**: Send document content to the LLM with a structured prompt requesting extraction of decisions, facts, patterns, and context. The prompt instructs the LLM to output JSON with fields: `tag`, `category`, `confidence`, `quality_flags`, `sources`, `content`.
4. **Quality analysis**: Part of the LLM prompt — instruct the LLM to detect missing_rationale, implied_assumption, incongruent, etc. This is not a separate pass.
5. **Write knowledge files**: For each extracted piece, write a markdown file with YAML frontmatter to the store's output directory.
6. **Update checkpoint**: Record the curation timestamp per source-store pair.

**LLM interface**: Uses `llm.Synthesizer` from spec 013. The `Synthesize(ctx, prompt) (string, error)` method sends the prompt and returns the LLM's response. The curation pipeline parses the JSON response.

**MCP mode**: When called via MCP tool (no local LLM), the tool returns the extraction prompt as structured output for the calling agent to perform synthesis — same pattern as `tools/compile.go` with nil synthesizer.

### R4: Incremental Curation Checkpoints

**Question**: How does incremental curation track which documents have been processed?

**Finding**: Use a lightweight state file in the store's directory: `.uf/dewey/knowledge/{store-name}/.curation-state.json`:

```json
{
  "last_curated_at": "2026-04-21T10:30:00Z",
  "source_checkpoints": {
    "disk-meetings": "2026-04-21T10:30:00Z",
    "disk-slack-export": "2026-04-20T15:00:00Z"
  }
}
```

The curation pipeline queries `store.ListPagesBySource(sourceID)` and filters to pages with `updated_at > last_curated_at` for that source. After successful curation, the checkpoint is updated.

**Alternative considered**: Store checkpoints in the SQLite `metadata` table. Rejected because the checkpoint is per-store, and stores are configured in YAML, not in the database. Keeping the state file alongside the knowledge files maintains the principle that knowledge stores are self-contained directories.

### R5: Background Curation Goroutine Pattern

**Question**: How does background curation integrate with the existing server lifecycle?

**Finding**: The pattern is established by spec 012 (background indexing):

```go
// In executeServe(), after background indexing goroutine:
if len(knowledgeStores) > 0 {
    go func() {
        for _, store := range knowledgeStores {
            ticker := time.NewTicker(store.CurationInterval)
            defer ticker.Stop()
            for {
                select {
                case <-ctx.Done():
                    return
                case <-ticker.C:
                    indexMu.Lock()
                    // curate incrementally
                    indexMu.Unlock()
                }
            }
        }
    }()
}
```

**Key constraints**:
- Must acquire `indexMu` before curating (FR-020) — same mutex used by index/reindex tools.
- Must respect `ctx.Done()` for graceful shutdown.
- Must log errors and continue polling (FR-021) — never crash the goroutine.
- Must wait for `indexReady` to be true before first curation run — curating before indexing completes would process stale data.

### R6: Curated Trust Tier

**Question**: Does adding `curated` as a tier value require a schema migration?

**Finding**: **No.** The `tier` column is `TEXT DEFAULT 'authored'` (added in v1→v2 migration, `store/migrate.go` line 93). It accepts any string value. The `curated` value is simply a new convention — no DDL change needed.

The `filterResultsByTier` function in `tools/semantic.go` (line 322) already does string comparison: `page.Tier == tier`. Adding `curated` as a value works immediately.

The `InsertPage` function in `store/store.go` (line 199) defaults tier to `"authored"` if empty. Curated pages will explicitly set `Tier: "curated"`.

### R7: Auto-Indexing Knowledge Store Directories

**Question**: How are knowledge store directories automatically registered as disk sources?

**Finding**: After curation writes files to the store's output directory, the pipeline registers the directory as a disk source using the existing `source.Manager` infrastructure:

1. Check if a source with ID `knowledge-{store-name}` already exists in the store.
2. If not, insert a new source record with `type: "disk"`, `name: "knowledge-{store-name}"`, and config `{"path": "<store-path>"}`.
3. Trigger indexing for that source (same as `dewey index --source knowledge-{store-name}`).

This ensures curated files are immediately searchable via `semantic_search`.

**Tier assignment**: When indexing curated files, the indexing pipeline must set `tier: "curated"` on the pages. This requires detecting the `knowledge-` source ID prefix in the indexing path and setting the tier accordingly.

---

## Phase 1: Design

### Quickstart

See [quickstart.md](quickstart.md) for the minimal viable path.

### Contracts

See [contracts/](contracts/) for:
- `curate-package.md` — `curate.StoreConfig`, `curate.Pipeline`, `curate.KnowledgeFile` types and methods
- `mcp-tools.md` — `curate` MCP tool input/output schema
- `cli-commands.md` — `dewey curate` command flags and output

### Data Flow

```
sources.yaml sources → dewey index → graph.db pages/blocks
                                          ↓
knowledge-stores.yaml → curate pipeline → LLM extraction
                                          ↓
                              .uf/dewey/knowledge/{store}/
                                   ↓              ↓
                              auto-index     quality flags
                                   ↓              ↓
                              graph.db        dewey lint
                              (tier: curated)  (metrics)
```

### Key Type Definitions

```go
// curate/config.go
type StoreConfig struct {
    Name             string   `yaml:"name"`
    Sources          []string `yaml:"sources"`
    Path             string   `yaml:"path"`
    CurateOnIndex    bool     `yaml:"curate_on_index"`
    CurationInterval string   `yaml:"curation_interval"`
}

// curate/curate.go
type QualityFlag struct {
    Type        string   `json:"type" yaml:"type"`         // missing_rationale, implied_assumption, incongruent, unsupported_claim
    Detail      string   `json:"detail" yaml:"detail"`
    Sources     []string `json:"sources" yaml:"sources"`   // source references
    Resolution  string   `json:"resolution,omitempty" yaml:"resolution,omitempty"`
}

type KnowledgeFile struct {
    Tag         string        `yaml:"tag"`
    Category    string        `yaml:"category"`     // decision, pattern, gotcha, context, reference
    Confidence  string        `yaml:"confidence"`   // high, medium, low, flagged
    QualityFlags []QualityFlag `yaml:"quality_flags,omitempty"`
    Sources     []SourceRef   `yaml:"sources"`
    StoreName   string        `yaml:"store"`
    CreatedAt   string        `yaml:"created_at"`
    Tier        string        `yaml:"tier"`          // always "curated"
    Content     string        `yaml:"-"`             // markdown body (not in frontmatter)
}

type SourceRef struct {
    SourceID   string `yaml:"source_id"`
    Document   string `yaml:"document"`
    Section    string `yaml:"section,omitempty"`
    Excerpt    string `yaml:"excerpt,omitempty"`
}

type CurationState struct {
    LastCuratedAt     time.Time            `json:"last_curated_at"`
    SourceCheckpoints map[string]time.Time `json:"source_checkpoints"`
}

// Pipeline is the main curation engine.
type Pipeline struct {
    store     *store.Store
    synth     llm.Synthesizer
    embedder  embed.Embedder
    vaultPath string
}
```

### Implementation Phases (for tasks.md)

**Phase 1: Foundation (US1 — File-Backed Learning Persistence)**
- Modify `tools/learning.go` to dual-write markdown files
- Add re-ingestion logic to `main.go` startup
- Tests for dual-write and re-ingestion

**Phase 2: Configuration (US2 — Knowledge Store Configuration)**
- Create `curate/config.go` for `knowledge-stores.yaml` parsing
- Extend `dewey init` to scaffold `knowledge-stores.yaml`
- Config validation and tests

**Phase 3: Curation Pipeline (US3, US4 — Extraction + Quality Analysis)**
- Create `curate/curate.go` with `Pipeline` struct
- LLM prompt engineering for extraction and quality analysis
- Knowledge file writing with YAML frontmatter
- MCP tool handler in `tools/curate.go`
- CLI command `dewey curate`
- Tests with `llm.NoopSynthesizer`

**Phase 4: Trust Tier + Search (US6 — Curated Tier)**
- Verify `curated` tier works in `semantic_search_filtered`
- Set tier on curated pages during indexing
- Tests for tier filtering

**Phase 5: Background Curation (US5 — Continuous Background Curation)**
- Background goroutine in `main.go`
- Incremental curation with checkpoints
- Mutex integration with indexing
- Tests for background lifecycle

**Phase 6: Lint + Auto-Index (US7, US8)**
- Extend `tools/lint.go` with knowledge store quality metrics
- Auto-register knowledge store directories as disk sources
- Tests for lint metrics and auto-indexing

**Phase 7: Integration + Documentation**
- End-to-end integration tests
- Update `AGENTS.md`, `README.md`
- Slash command for `dewey curate`
- Website documentation sync issue

### Coverage Strategy

| Package | Strategy | Target |
|---------|----------|--------|
| `curate/` | Contract tests: config parsing, pipeline extraction, file writing, quality analysis | ≥70% contract coverage |
| `tools/learning.go` | Unit tests: dual-write creates file, re-ingestion recovers learnings | Existing + new assertions |
| `tools/curate.go` | MCP tool tests: input validation, error cases, structured output | Same pattern as `tools/compile_test.go` |
| `tools/lint.go` | Unit tests: new knowledge store quality checks | Existing + new assertions |
| `tools/semantic.go` | Verify curated tier filtering (may already work) | Existing tests sufficient |
| `store/store.go` | Unit tests: new checkpoint queries | Existing + new assertions |
| `main.go` | Integration test: background curation lifecycle | Manual verification acceptable for goroutine lifecycle |

### Risk Assessment

| Risk | Likelihood | Impact | Mitigation |
|------|-----------|--------|------------|
| LLM extraction quality varies | High | Medium | Structured JSON prompt with examples. Quality flags catch issues. |
| Background curation blocks MCP tools | Medium | High | Mutex is held only during curation, not during LLM calls. LLM calls are the slow part. |
| File-backed learnings create merge conflicts in git | Low | Low | Each learning is a separate file. Conflicts only if two agents store the same `{tag}-{seq}` simultaneously. |
| Large knowledge stores slow down `dewey lint` | Low | Medium | Lint queries are SQL-based, not full-text. Index on tier column exists. |
| Curation checkpoint state file corruption | Low | Low | If corrupted, full re-curation runs (safe — idempotent). |

### Dependencies on Prior Specs

| Spec | Dependency | Status |
|------|-----------|--------|
| 008 (store_learning) | `tools/learning.go`, `store.NextLearningSequence` | ✅ Implemented |
| 011 (live-reindex) | `indexMu` mutex for mutual exclusion | ✅ Implemented |
| 012 (background-index) | Background goroutine pattern, `indexReady` flag | ✅ Implemented |
| 013 (knowledge-compile) | `llm.Synthesizer` interface, trust tiers, `tier` column | ✅ Implemented |
