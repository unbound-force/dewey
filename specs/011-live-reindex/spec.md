# Feature Specification: Live Reindex

**Feature Branch**: `011-live-reindex`
**Created**: 2026-04-07
**Status**: Draft
**Input**: MCP tools and slash commands for index and reindex operations while dewey serve is running

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Agent-Triggered Incremental Index (Priority: P1)

An AI agent is working in a project where the developer recently added a new web source to `sources.yaml` or a sibling repo has been updated with new specs. The agent needs fresh content in Dewey's index but cannot open a terminal — it's mid-task in an OpenCode session. The agent calls the `index` MCP tool, which fetches all configured sources, indexes new and changed content, generates embeddings, and returns a summary of what was updated. The agent can then immediately search the newly-indexed content.

**Why this priority**: This is the core use case — agents need to refresh the index without human intervention or terminal access. Every other story builds on this capability.

**Independent Test**: Configure a disk source pointing to a directory. Add a new markdown file to that directory. Call the `index` MCP tool. Verify the new file appears in subsequent `semantic_search` results.

**Acceptance Scenarios**:

1. **Given** a running `dewey serve` with configured sources, **When** an agent calls the `index` tool, **Then** all configured sources are fetched, new and changed content is indexed with embeddings, and the tool returns a summary with counts (sources processed, pages new/changed/deleted, embeddings generated, elapsed time)
2. **Given** a running `dewey serve` with no configured sources, **When** an agent calls the `index` tool, **Then** the tool returns a summary indicating zero sources processed (no error)
3. **Given** a running `dewey serve`, **When** an agent calls the `index` tool with a `source_id` parameter, **Then** only that specific source is re-indexed (others are untouched)

---

### User Story 2 - Agent-Triggered Full Reindex (Priority: P2)

An AI agent determines that the index is stale or corrupted — perhaps `dewey doctor` reported inconsistencies, or search results are returning outdated content. The agent calls the `reindex` MCP tool, which deletes the existing index data for external sources and rebuilds it from scratch. This is the "nuclear option" that guarantees a clean state. The local vault content (pages loaded by `dewey serve` at startup) is not affected — only external source content is rebuilt.

**Why this priority**: Full reindex is less common than incremental but essential when the index is in a bad state. It must be available without terminal access.

**Independent Test**: Index a source, then delete a file from the source directory. Call `reindex`. Verify the deleted file's page is removed from the index.

**Acceptance Scenarios**:

1. **Given** a running `dewey serve` with previously-indexed external sources, **When** an agent calls the `reindex` tool, **Then** all external source content is deleted and re-indexed from scratch, and the tool returns a rebuild summary
2. **Given** a running `dewey serve`, **When** an agent calls `reindex` while the `index` tool is already running, **Then** the second call returns an error indicating an operation is already in progress
3. **Given** a running `dewey serve`, **When** an agent calls `reindex`, **Then** non-semantic MCP tools (search, get_page, traverse) remain functional during the reindex operation — the server does not block

---

### User Story 3 - Slash Command Fallback (Priority: P3)

A developer is in an OpenCode session and wants to trigger an index refresh without switching to a terminal. They type `/dewey-index` or `/dewey-reindex` in the OpenCode prompt. The slash command instructs the agent to call the corresponding MCP tool. This provides a human-friendly entry point that delegates to the MCP tool — the slash command is a thin wrapper, not a separate implementation.

**Why this priority**: Slash commands are a convenience layer for humans. The MCP tools (US1/US2) are the foundation — slash commands just make them discoverable via the command palette.

**Independent Test**: Type `/dewey-index` in OpenCode. Verify the agent calls the `index` MCP tool and displays the summary.

**Acceptance Scenarios**:

1. **Given** a developer in an OpenCode session, **When** they type `/dewey-index`, **Then** the agent calls the `index` MCP tool and displays the returned summary
2. **Given** a developer in an OpenCode session, **When** they type `/dewey-reindex`, **Then** the agent calls the `reindex` MCP tool and displays the rebuild summary with a warning that this deletes and rebuilds all external source content

---

### Edge Cases

- What happens when indexing takes longer than expected (e.g., web crawl with slow responses)? The MCP tool runs to completion — there is no timeout on the tool call itself. The server remains responsive to other tool calls during indexing.
- What happens when Ollama is unavailable during index? Content is indexed without embeddings. The tool's response indicates how many pages were indexed without embeddings, matching the existing `--no-embeddings` degradation pattern.
- What happens when sources.yaml is malformed? The tool returns an error with the parse failure message. No content is modified.
- What happens when the persistent store is not configured (in-memory mode)? The tool returns an error explaining that persistent storage is required for indexing.
- What happens when a reindex is called while another reindex is running? The second call returns an error indicating an operation is already in progress (mutual exclusion).

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: Dewey MUST provide an `index` MCP tool that fetches all configured external sources, indexes new and changed content, generates embeddings (when available), and returns a structured summary
- **FR-002**: The `index` tool MUST accept an optional `source_id` parameter to re-index only a specific source
- **FR-003**: Dewey MUST provide a `reindex` MCP tool that deletes all external source content from the store and re-indexes from scratch
- **FR-004**: Both tools MUST return a structured summary including: sources processed, pages new/changed/deleted, embeddings generated, elapsed time
- **FR-005**: Both tools MUST be mutually exclusive — only one indexing operation can run at a time. Concurrent calls MUST return an error
- **FR-006**: The server MUST remain responsive to non-indexing MCP tool calls (search, get_page, traverse, etc.) while an indexing operation is in progress
- **FR-007**: When Ollama is unavailable, indexing MUST proceed without embeddings and report the count of pages indexed without embeddings in the summary
- **FR-008**: Both tools MUST require persistent storage — they return an error when the store is not configured
- **FR-009**: The `reindex` tool MUST NOT affect local vault content (pages indexed by `IncrementalIndex` at serve startup) — only external source content is rebuilt
- **FR-010**: OpenCode slash command definitions MUST exist at `.opencode/command/dewey-index.md` and `.opencode/command/dewey-reindex.md` that instruct the agent to call the corresponding MCP tools
- **FR-011**: Both tools MUST log their progress using the structured diagnostics format (ISO 8601 timestamps, component prefix, elapsed timing) established in spec 009

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: An agent can trigger a source re-index and query newly-indexed content within a single session — round-trip from tool call to searchable content under 30 seconds for a typical source (100 files)
- **SC-002**: A developer can type `/dewey-index` in OpenCode and see a summary of what was indexed without switching to a terminal
- **SC-003**: The `reindex` tool produces a clean rebuild that matches the output of running `dewey reindex` from the CLI — same page counts, same embedding coverage
- **SC-004**: All existing MCP tools continue to function during an indexing operation — no blocked calls, no errors, no stale data mid-operation
- **SC-005**: All existing tests continue to pass — the new tools do not affect existing behavior

## Assumptions

- The `index` and `reindex` MCP tools reuse the existing source fetching and indexing infrastructure from `source.Manager` and the store persistence pipeline (`PersistBlocks`, `PersistLinks`, `GenerateEmbeddings`). No new indexing logic is created — the tools wrap the existing pipeline.
- Mutual exclusion is enforced with a simple mutex or atomic flag on the server — the first indexing operation acquires the lock, subsequent calls receive an error until it completes.
- The `reindex` tool deletes external source content by calling `store.DeletePagesBySource()` for each configured source ID, then re-indexes. It does NOT delete pages with `source_id = "disk-local"` (vault content) or `source_id = "learning"` (stored learnings).
- Slash commands are thin wrappers — they contain instructions for the agent to call the MCP tool, not implementation logic. They are `.md` files in `.opencode/command/` following the existing convention.
- The local vault's `IncrementalIndex` (which runs at serve startup) is separate from the external source indexing triggered by these tools. The two do not interfere.
