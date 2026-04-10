# Feature Specification: Knowledge Compilation & Temporal Intelligence

**Feature Branch**: `013-knowledge-compile`
**Created**: 2026-04-10
**Status**: Draft
**Input**: Knowledge compilation with temporal awareness, autonomous LLM synthesis, linting, contamination separation, and knowledge evolution — inspired by Karpathy's LLM Knowledge Base approach

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Temporal Awareness in Learnings (Priority: P1)

An AI agent completes a task and stores a learning: "Use Option A for authentication with a 30-second timeout." Three sessions later, the same project switches approaches: "Switch to Option B for authentication because Option A had rate limiting issues." Today, both learnings exist in the index with equal weight — a future agent searching for "authentication" finds both and has no way to know which is current without reading both and reasoning about the contradiction.

With temporal awareness, every learning has a creation timestamp and a topic tag. The agent stores learnings with `tag: "authentication"` and `category: "decision"`. Search results include `created_at` metadata so agents can see which is newer. The learning identity is `{tag}-{sequence}` (e.g., `authentication-1`, `authentication-2`), giving every learning a human-readable, addressable name.

**Why this priority**: Temporal ordering is the foundation that compilation, linting, and contamination separation all depend on. Without knowing which learning is newer, no other feature can resolve contradictions.

**Independent Test**: Store two learnings with the same tag. Query for that tag. Verify both appear with `created_at` timestamps and `{tag}-{sequence}` identities, and that the newer one has a later timestamp.

**Acceptance Scenarios**:

1. **Given** a running Dewey instance, **When** an agent calls `store_learning` with `tag: "authentication"` and `category: "decision"`, **Then** the learning is stored with a `created_at` timestamp and identity `authentication-1`
2. **Given** a learning `authentication-1` exists, **When** an agent stores another learning with `tag: "authentication"`, **Then** it receives identity `authentication-2` (auto-incremented)
3. **Given** multiple learnings exist, **When** an agent calls `semantic_search` with terms matching those learnings, **Then** each result includes `created_at` in its metadata
4. **Given** learnings with different categories (decision, pattern, gotcha), **When** an agent queries, **Then** results include the `category` in metadata

---

### User Story 2 - Knowledge Compilation (Priority: P1)

A developer has been working on a project for two weeks. Across 15 sessions, agents have stored 40 learnings — decisions about architecture, gotchas about specific files, patterns that worked, context about constraints. These learnings are scattered, sometimes contradictory (early decisions were revised), and hard to navigate. The developer runs `dewey compile` and Dewey reads all stored learnings, clusters them by topic, and produces a set of compiled articles — one per topic cluster. Each article presents the **current truth**: facts from newer learnings replace contradicted facts from older ones, while non-contradicted information carries forward unchanged. A history section preserves the full evolution timeline.

The compiled articles are regular markdown files in `.uf/dewey/compiled/`. They are automatically searchable alongside specs, code, and other indexed content. When an agent in session 16 searches for "authentication," it finds the compiled article that says "Use Option B (changed from A in session 5 due to rate limiting). Timeout: 60s (increased from 30s in session 8). Rate limit: 100 req/min" — a single, authoritative, current-state document.

**Why this priority**: This is the core Karpathy insight — the LLM acts as a librarian that synthesizes raw materials into a curated wiki. Without compilation, learnings accumulate but never consolidate. Compilation is what transforms a log of events into actionable knowledge.

**Independent Test**: Store 5 learnings on the same topic across different sessions, including one that contradicts an earlier one. Run `dewey compile`. Verify a compiled article exists that merges all non-contradicted facts and resolves the contradiction in favor of the newer learning.

**Acceptance Scenarios**:

1. **Given** 10 stored learnings across 3 topics, **When** the developer runs `dewey compile`, **Then** compiled articles are produced in `.uf/dewey/compiled/` — one per topic cluster, plus an index file
2. **Given** two learnings on the same topic where the newer one contradicts a fact in the older one, **When** compilation runs, **Then** the compiled article presents the newer fact as current truth and preserves the older fact in the history section
3. **Given** two learnings on the same topic where the newer one adds information without contradicting the older one, **When** compilation runs, **Then** the compiled article includes both facts (additive merge)
4. **Given** learnings with `category: "decision"`, **When** compilation runs, **Then** decisions get temporal resolution (newer wins) while learnings with `category: "pattern"` are accumulated (multiple patterns are additive)
5. **Given** compiled articles exist, **When** an agent calls `semantic_search`, **Then** compiled articles appear in results alongside other content
6. **Given** a configurable LLM provider, **When** the developer runs `dewey compile`, **Then** the configured model is used for synthesis (default: opencode session model)

---

### User Story 3 - Session-End Auto-Compile (Priority: P1)

The `/unleash` pipeline's retrospective step stores learnings after each session. Today, those learnings accumulate but are never synthesized. With session-end auto-compile, the retrospective step triggers an incremental compilation after storing learnings — only the new learnings from this session are merged into existing compiled articles (or a new article is created if the topic is new). This ensures the compiled knowledge base stays current without manual intervention.

**Why this priority**: Compilation is only valuable if it happens. An on-demand-only compile step will be forgotten. Session-end triggering makes knowledge evolution automatic — every session leaves the knowledge base better than it found it.

**Independent Test**: Run `/unleash` on a feature. Verify the retrospective step stores learnings AND triggers incremental compilation. Verify compiled articles are updated with the new learnings.

**Acceptance Scenarios**:

1. **Given** the `/unleash` retrospective step stores 2 new learnings, **When** the retrospective completes, **Then** incremental compilation runs automatically on those 2 learnings
2. **Given** session-end compilation fails (LLM unavailable), **When** the failure occurs, **Then** the retrospective still completes successfully — compilation failure is non-blocking
3. **Given** session-end compilation runs, **When** a new learning doesn't match any existing compiled article topic, **Then** a new compiled article is created for that topic

---

### User Story 4 - Knowledge Linting (Priority: P2)

A developer suspects their knowledge base has quality issues — stale decisions that haven't been revisited in weeks, learnings that were stored but never compiled, embedding gaps from when Ollama was unavailable. The developer runs `dewey lint` and gets a structured report: 3 stale decisions (>30 days old), 12 learnings not yet compiled, 5 pages missing embeddings, 2 pairs of potentially contradicting learnings. For mechanical issues (missing embeddings), `dewey lint --fix` auto-repairs them. For semantic issues (contradictions, staleness), the report suggests running `dewey compile` or reviewing specific learnings.

**Why this priority**: Linting is the "health check" for knowledge quality. Without it, the knowledge base degrades silently — stale decisions persist, contradictions accumulate, and embedding gaps reduce search coverage.

**Independent Test**: Store a learning tagged `decision` and wait (or backdate its timestamp). Run `dewey lint`. Verify it reports the learning as stale.

**Acceptance Scenarios**:

1. **Given** a learning with `category: "decision"` older than 30 days, **When** `dewey lint` runs, **Then** it reports the learning as a stale decision
2. **Given** stored learnings that have not been compiled, **When** `dewey lint` runs, **Then** it reports the count of uncompiled learnings
3. **Given** pages without embeddings, **When** `dewey lint --fix` runs, **Then** embeddings are generated for those pages
4. **Given** two learnings with high semantic similarity but different conclusions, **When** `dewey lint` runs, **Then** it flags them as potentially contradicting

---

### User Story 5 - Contamination Separation (Priority: P2)

A project's Dewey index contains human-authored specs (reviewed, precise), agent-generated learnings (useful but unreviewed), and compiled articles (LLM-synthesized summaries). An agent searching for context finds all three mixed together with equal weight. The agent can't distinguish "this is a reviewed spec" from "this is a guess from a previous session's retrospective."

With contamination separation, every page has a trust tier: `authored` (human-written sources from disk/GitHub/web/code), `draft` (raw learnings and fresh compiled articles), or `validated` (agent content promoted by human review). Agents can filter search results by tier when they need high-confidence context, or include all tiers when exploring broadly.

**Why this priority**: As `store_learning` and `dewey compile` generate more agent content, distinguishing it from human-authored content becomes essential for trust. Lower priority than compilation because it's only valuable once there's enough agent content to need filtering.

**Independent Test**: Store a learning (tier: draft). Search with tier filter for "authored" only. Verify the learning does not appear. Search without filter. Verify it appears.

**Acceptance Scenarios**:

1. **Given** a stored learning, **When** it is created, **Then** its trust tier is `draft`
2. **Given** a compiled article, **When** it is created, **Then** its trust tier is `draft`
3. **Given** pages from disk/GitHub/web/code sources, **When** they are indexed, **Then** their trust tier is `authored`
4. **Given** a draft learning, **When** a developer runs `dewey promote` on it, **Then** its tier changes to `validated`
5. **Given** pages with different tiers, **When** an agent calls `semantic_search_filtered` with a tier filter, **Then** only pages matching that tier are returned

---

### User Story 6 - Knowledge Evolution in Compilation (Priority: P3)

Over time, learnings on the same topic evolve. Session 1: "Use Option A for auth. Timeout 30s. Rate limit 100/min." Session 5: "Switch to Option B due to rate limiting." Session 8: "Increase timeout to 60s per user feedback." The compilation step detects that "Switch to Option B" modifies the "Use Option A" fact but does NOT affect the rate limit fact. The compiled article merges all three learnings into a current-state document: "Use Option B (changed from A, session 5). Timeout: 60s (increased from 30s, session 8). Rate limit: 100/min (unchanged)." Non-contradicted facts carry forward. Superseded facts are replaced with current values. A history section preserves the full evolution.

**Why this priority**: This is the most sophisticated compilation behavior — partial supersession where some facts are replaced while others carry forward. It depends on US1 (temporal ordering) and US2 (basic compilation) being in place first.

**Independent Test**: Store 3 learnings on the same topic where the 2nd modifies one fact from the 1st and the 3rd modifies a different fact. Run `dewey compile`. Verify the compiled article has all three facts current (two modified, one carried forward) plus a history section.

**Acceptance Scenarios**:

1. **Given** 3 learnings where #2 modifies a fact from #1 and #3 modifies a different fact from #1, **When** compilation runs, **Then** the compiled article presents all facts at their most recent values
2. **Given** a fact that was never contradicted across 5 learnings, **When** compilation runs, **Then** the fact appears in the current-state section without modification
3. **Given** compilation produces a current-state article, **When** the article is examined, **Then** it includes a history section with the chronological evolution of each modified fact

---

### Edge Cases

- What happens when no learnings exist? `dewey compile` produces an empty compiled directory with just an `_index.md` noting "No learnings to compile." No error.
- What happens when the LLM is unavailable during compilation? The compile step fails with a clear error. Existing compiled articles are not affected. The failure is logged. Session-end auto-compile treats this as non-blocking.
- What happens when two learnings on the same topic have the same timestamp? They are treated as additive (both facts included). Compilation does not attempt to determine which came "first" within the same timestamp.
- What happens when `dewey lint --fix` generates embeddings for a page that has since been deleted from the source? The embedding is generated for the page as it exists in the store. If the page is later removed by a reindex, the embedding is cleaned up normally.
- What happens when a developer promotes a draft learning and then a newer learning contradicts it? The validated status is informational — it does not prevent supersession during compilation. The compiled article resolves contradictions by recency regardless of tier.
- What happens when the compiled article taxonomy changes between full rebuilds? The old compiled directory is replaced entirely. Compiled articles are ephemeral — the learnings are the source of truth, compiled articles are the materialized view.

## Requirements *(mandatory)*

### Functional Requirements

**Temporal Awareness**

- **FR-001**: `store_learning` MUST accept a required `tag` parameter (topic string) and an optional `category` parameter (one of: `decision`, `pattern`, `gotcha`, `context`, `reference`)
- **FR-002**: Each stored learning MUST be assigned an auto-incremented identity in the format `{tag}-{sequence}` (e.g., `authentication-3`), unique within its tag namespace
- **FR-003**: Each stored learning MUST include a `created_at` property with ISO 8601 timestamp, set automatically at storage time
- **FR-004**: `semantic_search` results MUST include `created_at` and `category` in result metadata when available
- **FR-005**: The `store_learning` return value MUST include the assigned `{tag}-{sequence}` identity

**Compilation**

- **FR-006**: Dewey MUST provide a `compile` CLI command and MCP tool that reads all stored learnings, clusters them by topic, and produces synthesized articles
- **FR-007**: Compilation MUST use tag-assisted semantic clustering — learnings with shared tags are grouped first, then refined by semantic similarity
- **FR-008**: Compilation MUST apply category-aware resolution: `decision` learnings get temporal merge (newer wins, non-contradicted facts carry forward), `pattern` learnings accumulate, `gotcha` learnings de-duplicate, `context` carries forward, `reference` is preserved as-is
- **FR-009**: The compiler MUST discover the file taxonomy — one article per topic cluster, auto-named based on dominant tags, organized under `.uf/dewey/compiled/` with an auto-generated `_index.md`
- **FR-010**: Each compiled article MUST include a current-state section (merged truth) and a history section (chronological evolution)
- **FR-011**: The compilation LLM provider MUST be configurable. The default MUST use the opencode session's configured model
- **FR-012**: Compiled articles MUST be standard markdown files automatically indexable by the existing store pipeline

**Session-End Integration**

- **FR-013**: The `/unleash` retrospective step MUST trigger incremental compilation after storing learnings — only new learnings are merged into existing compiled articles
- **FR-014**: Session-end compilation failure MUST be non-blocking — the retrospective completes regardless
- **FR-015**: If no existing compiled article matches the new learning's topic, a new compiled article MUST be created

**Linting**

- **FR-016**: Dewey MUST provide a `lint` CLI command and MCP tool that scans the index for knowledge quality issues
- **FR-017**: `dewey lint` MUST detect: stale decisions (>30 days old, not validated), uncompiled learnings, embedding gaps, and potentially contradicting learnings (high semantic similarity, different conclusions)
- **FR-018**: `dewey lint --fix` MUST auto-repair mechanical issues (regenerate missing embeddings) without LLM involvement
- **FR-019**: Lint results MUST suggest actionable remediation for each finding (e.g., "run dewey compile" for uncompiled learnings, "review these 2 contradicting learnings" for contradictions)

**Contamination Separation**

- **FR-020**: The persistent store MUST support a trust tier for each page: `authored` (human-written sources), `validated` (promoted agent content), or `draft` (raw learnings and fresh compiled articles)
- **FR-021**: Learnings stored via `store_learning` MUST default to `draft` tier
- **FR-022**: Pages from disk, GitHub, web, and code sources MUST default to `authored` tier
- **FR-023**: Dewey MUST provide a `promote` CLI command and MCP tool that changes a page's tier from `draft` to `validated`
- **FR-024**: `semantic_search_filtered` MUST support filtering by `tier` parameter

**Knowledge Evolution**

- **FR-025**: The compiler MUST detect when a newer learning modifies specific facts from an older learning without invalidating the entire older learning
- **FR-026**: Non-contradicted facts from older learnings MUST carry forward into the compiled article unchanged
- **FR-027**: The compiled article's history section MUST attribute each fact change to the session/learning that introduced it
- **FR-028**: A full rebuild (`dewey compile`) MUST produce the same compiled articles as an equivalent sequence of incremental compiles (deterministic output given the same learnings)

### Key Entities

- **Learning** (extended): A stored piece of knowledge with `tag` (topic namespace), `category` (decision/pattern/gotcha/context/reference), `created_at` (timestamp), `tier` (draft/validated/authored), and `{tag}-{sequence}` identity. The append-only event log.
- **Compiled Article**: A synthesized markdown document produced by `dewey compile` that presents the current truth for a topic cluster. Contains a current-state section and a history section. The materialized view.
- **Topic Cluster**: A compiler-discovered grouping of semantically related learnings. Determined by tag similarity first, refined by semantic embedding distance. Each cluster produces one compiled article.
- **Trust Tier**: One of `authored` (human sources), `validated` (promoted agent content), or `draft` (raw learnings and fresh compiled articles). Stored as a column in the persistent store, filterable in search.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: An agent can store a learning with a tag and category and receive a `{tag}-{sequence}` identity within 1 second
- **SC-002**: `dewey compile` produces compiled articles that resolve temporal contradictions — a search for a topic returns one authoritative current-state document instead of multiple conflicting learnings
- **SC-003**: After 15 sessions with 40 stored learnings, `dewey compile` produces a navigable set of compiled articles in under 60 seconds
- **SC-004**: Session-end auto-compile integrates new learnings into existing compiled articles without manual intervention — the knowledge base improves after every session
- **SC-005**: `dewey lint` identifies 100% of pages missing embeddings and 100% of learnings not yet compiled
- **SC-006**: An agent can filter search results by trust tier — querying for `tier: "authored"` returns zero agent-generated content
- **SC-007**: A compiled article about a topic with 3 evolving learnings correctly carries forward non-contradicted facts and replaces superseded facts with current values
- **SC-008**: All existing tests continue to pass — the `store_learning` API change (adding required `tag`) is backward-compatible via a default tag when not provided

## Assumptions

- The compilation LLM is capable of understanding temporal ordering and resolving factual contradictions in natural language. The prompt engineering for this is part of the implementation, not the spec.
- Tag-assisted semantic clustering uses the existing embedding infrastructure — learnings are already embedded, so clustering by cosine similarity is a database query, not a new pipeline.
- The `{tag}-{sequence}` identity is scoped to this Dewey instance. Cross-instance identity (e.g., sharing learnings between repos) is out of scope.
- The `store_learning` API change from `tags` (plural, optional, comma-separated) to `tag` (singular, required) is a breaking change. The existing stored learning (which has no tag) will receive a default tag of `general` during migration.
- Compiled articles are ephemeral — they are rebuilt from learnings and can be deleted and regenerated at any time. The learnings are the source of truth.
- The `category` field is optional with no default. Learnings without a category are treated as `context` during compilation (the safest default — carried forward unless contradicted).
- The `/unleash` command file (`.opencode/command/unleash.md`) is updated as part of this spec — adding the compile trigger to the retrospective step. This is a documentation change, not a code change, since `/unleash` is an agent instruction file.
- Dependencies: Spec 008 (`store_learning` tool), Spec 011 (`index`/`reindex` MCP tools — same tool registration pattern), Spec 012 (background indexing — same mutex pattern for compilation).
