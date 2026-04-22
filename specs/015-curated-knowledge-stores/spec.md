# Feature Specification: Curated Knowledge Stores

**Feature Branch**: `015-curated-knowledge-stores`
**Created**: 2026-04-21
**Status**: Draft
**Input**: Curated knowledge stores with source-mapped extraction, file-backed persistence, continuous background curation, quality analysis, confidence scoring, new curated trust tier, and automatic re-ingestion of file-backed learnings on startup (org discussion #114, issue #50)

## User Scenarios & Testing *(mandatory)*

### User Story 1 - File-Backed Learning Persistence (Priority: P1)

A developer has stored 40 learnings across 15 sessions via `store_learning`. The project's `graph.db` gets corrupted — perhaps a disk issue or an accidental deletion. Today, all 40 learnings are permanently lost. With file-backed persistence, every `store_learning` call dual-writes to both SQLite and a markdown file at `.uf/dewey/learnings/{tag}-{seq}.md`. When `dewey serve` restarts and detects orphaned markdown files not in the database, it automatically re-ingests them — all 40 learnings are recovered without any manual intervention.

**Why this priority**: This is the foundation everything else depends on. File-backed persistence is what makes learnings version-controllable, portable, and recoverable. Without it, all knowledge is locked in a single SQLite file.

**Independent Test**: Store 3 learnings. Delete `graph.db`. Restart `dewey serve`. Verify all 3 learnings are re-ingested from the markdown files and appear in search results.

**Acceptance Scenarios**:

1. **Given** a learning is stored via `store_learning`, **When** the operation completes, **Then** both a SQLite record AND a markdown file at `.uf/dewey/learnings/{tag}-{seq}.md` exist
2. **Given** a learning markdown file has YAML frontmatter with tag, category, created_at, and identity, **When** a human reads the file, **Then** they can understand the learning without accessing the database
3. **Given** `graph.db` is deleted but learning markdown files exist, **When** `dewey serve` starts, **Then** all orphaned learnings are automatically re-ingested into the new database
4. **Given** learning markdown files are committed to git, **When** a team member clones the repo and runs `dewey serve`, **Then** all learnings are available in their local index

---

### User Story 2 - Knowledge Store Configuration (Priority: P2)

A developer wants to automatically extract structured knowledge from meeting notes and Slack conversations that are indexed as Dewey sources. They create a `.uf/dewey/knowledge-stores.yaml` configuration that maps specific sources to named knowledge stores. Each store defines which sources feed into it, where the curated knowledge files are written, and how curation should behave.

**Why this priority**: Configuration is the contract between the user's intent ("curate knowledge from these sources") and the system's behavior. Without it, the curation pipeline has no instructions.

**Independent Test**: Create a `knowledge-stores.yaml` with one store mapping two sources. Run `dewey curate`. Verify knowledge files appear in the store's output directory.

**Acceptance Scenarios**:

1. **Given** a `knowledge-stores.yaml` defining a store named "team-decisions" with sources `[disk-meetings, disk-slack-export]`, **When** `dewey curate` runs, **Then** knowledge is extracted only from those two sources
2. **Given** a store with `curate_on_index: true`, **When** `dewey index` completes, **Then** curation runs automatically for that store
3. **Given** a store with no sources configured, **When** `dewey curate` runs, **Then** the store is skipped with an informational message
4. **Given** a store referencing a source ID that does not exist in `sources.yaml`, **When** the configuration is loaded, **Then** a warning is logged and the missing source is skipped

---

### User Story 3 - Source-Mapped Knowledge Extraction (Priority: P3)

An AI agent calls `dewey curate` (or it runs automatically after indexing). The curation pipeline reads indexed content from the configured sources, uses an LLM to extract key decisions, facts, patterns, and context, and writes each extracted piece as a markdown file in the knowledge store directory. Each file includes full source traceability — which source, which document, which section the knowledge was extracted from.

**Why this priority**: This is the core curation intelligence — the LLM reads raw source content and produces structured, tagged, source-traced knowledge. Without it, the knowledge store is an empty directory.

**Independent Test**: Index a source with meeting notes containing 3 decisions. Run `dewey curate`. Verify 3 knowledge files appear with correct tags, categories, source references, and content.

**Acceptance Scenarios**:

1. **Given** indexed meeting notes containing "Team decided to switch to OAuth2", **When** curation runs, **Then** a knowledge file is created with `tag: "authentication"`, `category: "decision"`, and the source document reference
2. **Given** a knowledge file is created, **When** a human or agent reads it, **Then** the `sources` field in the frontmatter traces back to the specific source document and block
3. **Given** two sources discuss the same topic, **When** curation runs, **Then** related content is aggregated into a single knowledge file with multiple source references
4. **Given** the LLM is unavailable during curation, **When** the failure occurs, **Then** the curation logs an error and existing knowledge files are not affected

---

### User Story 4 - Quality Analysis During Curation (Priority: P4)

During curation, the LLM does not just extract facts — it identifies quality issues in the source material. Missing information (decisions without rationale), implied information (unstated assumptions), and incongruent information (contradictions across sources) are flagged in the knowledge file's metadata. Each curated fact carries a confidence score based on source agreement, explicitness, and contradiction status.

**Why this priority**: Knowledge without quality assessment is unreliable. An agent needs to know "this fact is high confidence (multiple sources agree)" vs "this fact is flagged (contradicts another source)" to make informed decisions.

**Independent Test**: Index two sources where one says "30s timeout" and another says "60s timeout" for the same feature. Run `dewey curate`. Verify the knowledge file has a `quality_flags` entry of type `incongruent` with both sources referenced and a temporal resolution.

**Acceptance Scenarios**:

1. **Given** a source contains a decision without rationale, **When** curation extracts the decision, **Then** the knowledge file includes a `quality_flags` entry with type `missing_rationale`
2. **Given** two sources disagree on a fact, **When** curation processes both, **Then** the knowledge file flags the contradiction with type `incongruent`, lists both sources, and applies temporal resolution (newer wins)
3. **Given** a source implies an assumption without stating it, **When** curation extracts the related fact, **Then** the knowledge file includes a flag with type `implied_assumption`
4. **Given** a curated fact has no contradictions and multiple sources agree, **When** the confidence is assigned, **Then** it is `high`

---

### User Story 5 - Continuous Background Curation (Priority: P5)

While `dewey serve` is running, a background goroutine periodically checks for new indexed content that hasn't been curated yet. When new content is detected, the curation pipeline runs incrementally — only processing new or changed documents, not re-curating everything. This ensures the knowledge store stays current without manual intervention.

**Why this priority**: Continuous curation is what makes knowledge stores "alive" — they evolve as new meetings happen, new Slack conversations are exported, and new issues are filed. Without it, knowledge stores become stale between manual `dewey curate` runs.

**Independent Test**: Start `dewey serve` with a configured knowledge store. Add a new markdown file to a mapped source directory. Wait for the background curation interval. Verify a new knowledge file appears in the store.

**Acceptance Scenarios**:

1. **Given** `dewey serve` is running with a configured knowledge store, **When** new content is indexed from a mapped source, **Then** the background curation goroutine detects and curates the new content within the configured interval
2. **Given** background curation is running, **When** no new content is detected, **Then** no curation work is performed (idle — no wasted LLM tokens)
3. **Given** background curation fails (LLM unavailable), **When** the failure occurs, **Then** the error is logged and the goroutine continues polling (does not crash or stop)
4. **Given** the curation interval is configurable, **When** the user sets `curation_interval: 5m` in the store config, **Then** the background goroutine checks every 5 minutes

---

### User Story 6 - Curated Trust Tier (Priority: P6)

All content in Dewey's index has a trust tier. A new `curated` tier is introduced for machine-extracted knowledge from source content. This tier sits between `authored` (human-written) and `validated` (human-reviewed agent content) in the trust hierarchy: `authored > curated > validated > draft`. Agents can filter search results by tier to control the quality of context they receive.

**Why this priority**: As the knowledge store generates more machine-extracted content, distinguishing it from human-authored content and unreviewed agent learnings becomes essential for trust.

**Independent Test**: Curate a knowledge file. Query `semantic_search_filtered` with `tier: "curated"`. Verify only curated content appears. Query with `tier: "authored"`. Verify curated content does NOT appear.

**Acceptance Scenarios**:

1. **Given** a knowledge file is created by the curation pipeline, **When** it is indexed, **Then** its trust tier is `curated`
2. **Given** a learning stored via `store_learning`, **When** it is created, **Then** its trust tier remains `draft` (unchanged)
3. **Given** content from a disk source is indexed, **When** it is stored, **Then** its trust tier remains `authored` (unchanged)
4. **Given** an agent queries `semantic_search_filtered` with `tier: "curated"`, **Then** only knowledge store content is returned

---

### User Story 7 - Knowledge Store Lint Integration (Priority: P7)

`dewey lint` is extended to report knowledge store quality metrics. It surfaces aggregate statistics: number of curated facts at each confidence level, count of quality flags by type, stores with unprocessed source content, and potential contradictions awaiting resolution.

**Why this priority**: Without lint integration, the quality flags in individual knowledge files are invisible at the project level. Lint provides the aggregate view.

**Independent Test**: Curate knowledge with some low-confidence flags. Run `dewey lint`. Verify the report includes knowledge store quality metrics.

**Acceptance Scenarios**:

1. **Given** a knowledge store has 3 facts with `confidence: low`, **When** `dewey lint` runs, **Then** the report shows "3 low-confidence curated facts"
2. **Given** a knowledge store has unprocessed source content, **When** `dewey lint` runs, **Then** the report shows the count of documents awaiting curation
3. **Given** a knowledge store has `incongruent` quality flags, **When** `dewey lint` runs, **Then** the report lists the contradictions with source references

---

### User Story 8 - Auto-Indexed Knowledge Stores (Priority: P8)

Knowledge store directories are automatically registered as disk sources for Dewey's search index. When curated markdown files are written to `.uf/dewey/knowledge/<store>/`, they become immediately searchable via `semantic_search` alongside specs, code, and other indexed content — without the user manually adding a disk source entry to `sources.yaml`.

**Why this priority**: The knowledge store's value is in being searchable. If users have to manually configure a disk source for each store, the friction defeats the purpose.

**Independent Test**: Configure a knowledge store. Run curation. Query `semantic_search` for a topic covered by the curated content. Verify the curated knowledge file appears in results.

**Acceptance Scenarios**:

1. **Given** a knowledge store has curated files, **When** an agent queries `semantic_search` for a topic covered by the curated content, **Then** the curated knowledge appears in results with `tier: "curated"` in metadata
2. **Given** a new knowledge store is configured, **When** curation produces its first files, **Then** the store's directory is automatically indexed without manual `sources.yaml` changes

---

### Edge Cases

- What happens when two knowledge stores map the same source? Both stores curate from that source independently. Duplicate extraction is possible but not harmful — each store has its own context and may extract different knowledge based on its other sources.
- What happens when the curation LLM produces hallucinated facts not in the source? The quality analysis should catch this — facts without source references are flagged as `unsupported_claim`. The source traceability requirement means every fact must cite its origin.
- What happens when a learning markdown file is manually edited by a human? The edited version takes precedence on re-ingestion. The human edit is treated as a correction.
- What happens when curation runs on an empty source (no documents)? The store is skipped with an informational message. No files are created.
- What happens when the background curation goroutine and a manual `dewey curate` run simultaneously? They share the same mutex (similar to the index/reindex mutual exclusion from spec 011). The second caller gets an "already in progress" error.
- What happens when a source is removed from a store's configuration? Existing curated files from that source remain in the knowledge store. They are not automatically deleted — the user can manually clean them up. This prevents accidental knowledge loss.

## Requirements *(mandatory)*

### Functional Requirements

**File-Backed Learning Persistence**

- **FR-001**: `store_learning` MUST dual-write every learning to both SQLite and a markdown file at `.uf/dewey/learnings/{tag}-{seq}.md`
- **FR-002**: Learning markdown files MUST include YAML frontmatter with `tag`, `category`, `created_at`, `identity`, and `tier` fields
- **FR-003**: On `dewey serve` startup, the system MUST detect learning markdown files that exist on disk but not in the database and automatically re-ingest them
- **FR-004**: Re-ingestion MUST preserve the original `created_at`, `tag`, `category`, and `identity` from the frontmatter — not generate new values

**Knowledge Store Configuration**

- **FR-005**: Dewey MUST support a `.uf/dewey/knowledge-stores.yaml` configuration file defining named knowledge stores
- **FR-006**: Each knowledge store MUST specify: `name`, `sources` (list of source IDs from `sources.yaml`), `path` (output directory), and optional `curation_interval` (duration string for background polling)
- **FR-007**: `dewey init` MUST scaffold a default `knowledge-stores.yaml` with a commented-out example store

**Curation Pipeline**

- **FR-008**: Dewey MUST provide a `curate` CLI command and MCP tool that reads indexed content from configured sources and produces curated knowledge files
- **FR-009**: The curation pipeline MUST use a configurable LLM to extract decisions, facts, patterns, and context from source documents
- **FR-010**: Each curated knowledge file MUST include source traceability: source ID, document name, block/section reference, and relevant excerpt
- **FR-011**: The curation pipeline MUST apply temporal resolution — when newer source content contradicts older content, the newer fact takes precedence with the older fact preserved in history
- **FR-012**: Curated knowledge files MUST be written as markdown with YAML frontmatter to the store's configured `path` directory

**Quality Analysis**

- **FR-013**: The curation pipeline MUST detect and flag missing information: decisions without rationale, actions without owners, requirements without acceptance criteria, references to undefined terms
- **FR-014**: The curation pipeline MUST detect and flag implied information: unstated assumptions, inherited context, implicit dependencies, default values
- **FR-015**: The curation pipeline MUST detect and flag incongruent information: cross-source contradictions, stale references, scope conflicts, timeline conflicts
- **FR-016**: Each curated fact MUST carry a confidence score: `high` (explicit, no contradictions, multiple sources agree), `medium` (explicit but single-source), `low` (implied or contradictions exist), `flagged` (missing critical info or unresolvable contradictions)

**Continuous Background Curation**

- **FR-017**: When `dewey serve` is running, a background goroutine MUST periodically check for new indexed content in mapped sources and curate incrementally
- **FR-018**: The background curation interval MUST be configurable per store (default: 10 minutes)
- **FR-019**: Background curation MUST be incremental — only processing new or changed documents since the last curation run
- **FR-020**: Background curation MUST share the same mutex used by `dewey index`, `dewey reindex`, and `dewey curate` to prevent concurrent operations
- **FR-021**: Background curation failures MUST be logged and MUST NOT crash or stop the background goroutine

**Trust Tier**

- **FR-022**: The persistent store MUST support a `curated` trust tier value in addition to existing `authored`, `validated`, and `draft` tiers
- **FR-023**: Content produced by the curation pipeline MUST default to `curated` tier
- **FR-024**: `semantic_search_filtered` MUST support filtering by the `curated` tier

**Lint Integration**

- **FR-025**: `dewey lint` MUST report knowledge store quality metrics: curated facts per confidence level, quality flag counts by type, unprocessed source content count
- **FR-026**: `dewey lint` MUST report knowledge stores with stale content (sources updated since last curation)

**Auto-Indexing**

- **FR-027**: Knowledge store directories MUST be automatically registered as disk sources during `dewey serve` startup and after each curation run
- **FR-028**: Auto-registered sources MUST use `source_id` format `knowledge-{store-name}` to distinguish them from user-configured sources

### Key Entities

- **Knowledge Store**: A named collection of curated knowledge files, configured in `knowledge-stores.yaml`, fed by one or more indexed sources. Persisted as a directory of markdown files.
- **Curated Knowledge File**: A markdown file with YAML frontmatter containing a curated fact or decision. Includes tag, category, confidence score, quality flags, source traceability, and the knowledge content.
- **Quality Flag**: A structured metadata entry on a curated fact indicating a quality issue (missing_rationale, implied_assumption, incongruent, etc.) with detail, source reference, and optional resolution.
- **Confidence Score**: A classification (high/medium/low/flagged) indicating how trustworthy a curated fact is, based on source agreement, explicitness, and contradiction status.
- **Curation Checkpoint**: A record of which source documents have been curated in each run, enabling incremental curation (only process new/changed content).

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Deleting `graph.db` and restarting `dewey serve` recovers 100% of file-backed learnings — zero knowledge loss
- **SC-002**: An agent can discover curated team decisions by querying `semantic_search` — curated knowledge files appear in results alongside authored content
- **SC-003**: Every curated fact traces back to its source document — 100% of curated knowledge files have non-empty `sources` in their frontmatter
- **SC-004**: Background curation detects and processes new indexed content within 2x the configured interval (default: within 20 minutes of indexing)
- **SC-005**: `dewey lint` reports aggregate knowledge quality — confidence distribution and quality flag counts are visible in the lint report
- **SC-006**: Curated knowledge files are searchable by trust tier — `semantic_search_filtered(tier: "curated")` returns only knowledge store content
- **SC-007**: Quality analysis identifies contradictions — when two sources disagree on a fact, the curated file includes an `incongruent` quality flag with both source references
- **SC-008**: All existing tests continue to pass — the new `curated` tier, file-backed learnings, and background curation do not regress existing behavior

## Assumptions

- The curation LLM is the same configurable provider used by `dewey compile` (spec 013). Default: opencode session model for MCP tool calls, Ollama for CLI `dewey curate`.
- The `knowledge-stores.yaml` file follows the same pattern as `sources.yaml` — YAML list with typed entries. It is created by `dewey init` with a commented-out example.
- Background curation uses the same goroutine + mutex pattern as background indexing (spec 012) and the index/reindex tools (spec 011). The curation goroutine acquires the shared mutex before operating.
- The `curated` trust tier is stored in the same `tier` column added by spec 013. No new schema migration is needed — just a new allowed value.
- Incremental curation tracks "last curated" timestamps per source-store pair. Documents with `indexed_at` newer than the last curation timestamp are processed. This is stored in a lightweight state file in the store's directory.
- Learning file-backing (#50) is implemented first in the codebase order — it's the foundation that knowledge store file-backing extends.
- Dependencies: Spec 008 (store_learning), Spec 011 (index/reindex mutex), Spec 012 (background indexing goroutine pattern), Spec 013 (knowledge compilation, LLM interface, trust tiers).
