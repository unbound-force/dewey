# Feature Specification: Store Learning MCP Tool

**Feature Branch**: `008-store-learning`
**Created**: 2026-04-03
**Status**: Draft
**Input**: GitHub issue #25 — Add dewey_store_learning MCP tool for semantic memory (Spec 021 FR-007–FR-011)

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Store and Retrieve Learnings (Priority: P1)

An AI agent completes a complex task — fixing a subtle race condition, discovering an undocumented API constraint, or identifying a pattern that worked well during implementation. The agent calls `dewey_store_learning` with a natural language paragraph describing the learning and optional tags. The learning is persisted in Dewey's index with an embedding, making it immediately searchable via `dewey_semantic_search`. The next time any agent encounters a similar problem, the learning surfaces in search results alongside specs, code docs, and other indexed content.

**Why this priority**: This is the core capability — without store and retrieve, there is no semantic memory. This single story delivers the full value loop: store a learning, search for it later, find it.

**Independent Test**: Call `dewey_store_learning` with a learning about "scaffold patterns." Query `dewey_semantic_search` for "scaffold patterns." Verify the learning appears in results with provenance metadata indicating it's a learning document.

**Acceptance Scenarios**:

1. **Given** a running Dewey instance with embeddings enabled, **When** an agent calls `dewey_store_learning` with `information: "The vault walker must build its ignore matcher in New(), not Load(), because IncrementalIndex() runs before Load() on the persistent store path."`, **Then** the tool returns a UUID identifying the stored learning
2. **Given** a learning was just stored, **When** an agent calls `dewey_semantic_search` with `query: "ignore matcher lifecycle vault startup"`, **Then** the learning appears in the results with its content and provenance metadata
3. **Given** a learning was stored with `tags: "006-unified-ignore, gotcha, vault-walker"`, **When** an agent calls `dewey_semantic_search_filtered` with `has_tag: "gotcha"`, **Then** only learnings and other documents tagged "gotcha" appear in results

---

### User Story 2 - Learnings Persist Across Sessions (Priority: P2)

An agent stores a learning during one OpenCode session. The developer closes OpenCode, restarts it the next day, and starts a new session with a new agent. The new agent searches for context and finds the learning from the previous session. Learnings survive `dewey serve` restarts because they are persisted in the same SQLite store as all other indexed content.

**Why this priority**: Without persistence, learnings are lost when the MCP server restarts — defeating the purpose of a memory layer. This is the second most important capability after basic store/retrieve.

**Independent Test**: Store a learning. Restart `dewey serve`. Query for the learning. Verify it persists.

**Acceptance Scenarios**:

1. **Given** a learning was stored in a previous session, **When** `dewey serve` restarts and an agent queries `dewey_semantic_search` with terms related to the learning, **Then** the learning appears in results
2. **Given** a learning was stored, **When** `dewey reindex` is run, **Then** the learning is preserved (reindex only rebuilds disk-source content, not stored learnings)

---

### User Story 3 - Learnings Coexist with Other Content (Priority: P3)

A developer has indexed local Markdown files, GitHub issues, and web documentation in Dewey. When they store learnings, those learnings are searchable alongside all other content — not in a separate silo. A search for "authentication timeout" might return a specification, a GitHub issue, and a learning about a past debugging session — all in a single query. Learnings are distinguished from other documents by their provenance metadata, allowing agents to filter for learnings specifically when needed.

**Why this priority**: The value of unified search is that learnings enhance the existing knowledge graph rather than living in a separate system. This is what makes Dewey's approach superior to Swarm's separate Hivemind store.

**Independent Test**: Store a learning about "authentication timeout." Index a spec that mentions authentication. Query `dewey_semantic_search` for "authentication timeout." Verify both the learning and the spec appear in results. Query `dewey_semantic_search_filtered` with `source_type: "learning"` to verify learnings can be filtered specifically.

**Acceptance Scenarios**:

1. **Given** a learning and a spec document both discuss "authentication," **When** an agent queries `dewey_semantic_search` for "authentication timeout," **Then** both the learning and the spec appear in results, ranked by semantic similarity
2. **Given** multiple learnings and specs exist in the index, **When** an agent queries `dewey_semantic_search_filtered` with a source type filter for learnings, **Then** only learning documents are returned

---

### Edge Cases

- What happens when Ollama is unavailable (keyword-only mode)? The learning text is stored in the index but without an embedding. It will be findable via `dewey_search` (keyword search) but not via `dewey_semantic_search` until embeddings are generated. An informational message is returned with the UUID.
- What happens when the `information` parameter is empty? The tool returns an error: "information parameter is required and must not be empty."
- What happens when the same learning is stored twice? Two separate documents are created with different UUIDs. Deduplication is the agent's responsibility — Dewey does not merge or deduplicate learnings.
- What happens when `dewey reindex` is run? Learnings are preserved. Reindex only rebuilds content from disk/GitHub/web sources. Learnings have a distinct source type and are not affected by reindex operations.
- What happens when the store is not configured (in-memory mode)? The tool returns an error explaining that persistent storage is required for learnings.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: Dewey MUST provide an MCP tool named `dewey_store_learning` that accepts `information` (required string) and `tags` (optional comma-separated string) parameters
- **FR-002**: Stored learnings MUST be persisted in Dewey's index and be immediately searchable via `dewey_semantic_search` and `dewey_search`
- **FR-003**: Learnings MUST include provenance metadata distinguishing them from other document types — specifically a source type of "learning" that is visible in search results
- **FR-004**: Learnings MUST support tags for filtering via `dewey_semantic_search_filtered` using the existing `has_tag` parameter
- **FR-005**: The tool MUST generate embeddings for the learning text using the same embedding model and chunking strategy as other indexed content
- **FR-006**: The tool MUST return the UUID of the stored learning on success
- **FR-007**: The tool MUST return a clear error when `information` is empty, when the persistent store is not configured, or when any storage operation fails
- **FR-008**: Learnings MUST survive `dewey serve` restarts and `dewey reindex` operations
- **FR-009**: When Ollama is unavailable, the tool MUST store the learning text without embeddings and return an informational message indicating that semantic search will be unavailable until embeddings are generated
- **FR-010**: The `dewey_store_learning` tool MUST be registered alongside the existing 40 MCP tools in the server setup, bringing the total to 41

### Key Entities

- **Learning**: A stored piece of knowledge with natural language content, optional tags, provenance metadata (source type "learning"), and a vector embedding. Created by agents via the MCP tool and persisted in the same store as all other indexed content.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: An agent can store a learning and retrieve it via semantic search within the same session — round-trip time under 2 seconds
- **SC-002**: Learnings persist across `dewey serve` restarts — 100% of previously stored learnings are searchable after restart
- **SC-003**: Learnings appear in unified search results alongside specs, issues, and other content — no separate query endpoint needed
- **SC-004**: Learnings can be filtered specifically via source type metadata — `dewey_semantic_search_filtered(source_type: "learning")` returns only learnings
- **SC-005**: All existing 40 MCP tools continue to function identically after the new tool is added

## Assumptions

- Learnings are stored as pages in Dewey's existing store with a special source ID prefix (e.g., `learning-{branch}-{timestamp}`) that distinguishes them from disk/GitHub/web sources.
- The existing `dewey_semantic_search_filtered` tool's `source_type` parameter can filter by the learning source type without modification — the filter already supports arbitrary source type matching.
- Tags are stored as page properties (matching the existing property system) so `dewey_semantic_search_filtered(has_tag: "gotcha")` works without changes to the search infrastructure.
- The learning document is represented as a single page with one block containing the full learning text. This matches the existing page/block model and enables the block-level chunking and embedding pipeline to process it naturally.
- Deduplication of learnings is the agent's responsibility. Dewey stores every `dewey_store_learning` call as a new document.
