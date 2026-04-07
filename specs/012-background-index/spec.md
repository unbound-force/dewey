# Feature Specification: Background Index

**Feature Branch**: `012-background-index`
**Created**: 2026-04-07
**Status**: Draft
**Input**: Defer vault indexing to a background goroutine so the MCP server starts immediately (GitHub issue #36)

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Instant MCP Server Startup (Priority: P1)

A developer opens their project in OpenCode. The Dewey MCP server needs to start within OpenCode's 30-second timeout. Today, on repos with 255+ local pages, vault indexing (incremental index + embedding generation) takes ~32 seconds — exceeding the timeout. The MCP server is killed before it can accept any requests, leaving the developer with no Dewey tools at all.

With background indexing, the MCP server starts accepting tool calls within 1 second. Vault indexing runs in the background. Tools that read from the persistent store (search, get_page, traverse) work immediately using the previous session's indexed data. When background indexing completes, the in-memory index is refreshed and results reflect the latest content.

**Why this priority**: This is the core problem — without this fix, Dewey is unusable on repos larger than ~200 pages. Every other story is secondary to "the server must start."

**Independent Test**: Start `dewey serve` on a repo with 255+ pages. Verify the "server ready" log appears within 2 seconds. Verify `get_page` returns results while background indexing is still running.

**Acceptance Scenarios**:

1. **Given** a repo with 255+ local pages, **When** `dewey serve` starts, **Then** the MCP server accepts tool calls within 2 seconds of process start
2. **Given** `dewey serve` has just started and background indexing is in progress, **When** an agent calls `get_page`, `search`, or `traverse`, **Then** the tools return results from the persistent store (previous session's data)
3. **Given** `dewey serve` has just started and background indexing is in progress, **When** an agent calls `semantic_search`, **Then** the tool returns results from the previous session's embeddings (stale but functional)
4. **Given** background indexing completes, **When** an agent calls any search or navigation tool, **Then** results reflect the latest indexed content including newly added or modified files

---

### User Story 2 - Indexing Status Visibility (Priority: P2)

While background indexing is running, the developer (or an AI agent) needs to know that indexing is in progress and when it completes. The structured diagnostics log (spec 009) shows the background indexing start and completion with timing. The `dewey doctor` command reports whether background indexing is still running or has completed.

**Why this priority**: Without visibility into indexing status, developers and agents can't distinguish "stale results because indexing hasn't finished" from "results are wrong because something is broken."

**Independent Test**: Start `dewey serve`, immediately check the log file, verify "background indexing started" appears. Wait for indexing to complete, verify "background indexing complete" with elapsed time and page counts.

**Acceptance Scenarios**:

1. **Given** `dewey serve` starts with background indexing, **When** the log file is read, **Then** an "indexing started" message appears immediately after "server ready"
2. **Given** background indexing completes, **When** the log file is read, **Then** an "indexing complete" message appears with elapsed time, page counts (new/changed/unchanged), and embedding count
3. **Given** background indexing is in progress, **When** `dewey doctor` runs, **Then** the status section indicates indexing is still running

---

### User Story 3 - Mutual Exclusion with Live Reindex (Priority: P3)

The `index` and `reindex` MCP tools (spec 011) use a mutex to prevent concurrent indexing operations. Background startup indexing must participate in this same mutual exclusion. If an agent triggers a manual `index` or `reindex` while background indexing is still running, the manual operation waits for background indexing to complete (or returns an "in progress" error), rather than running concurrently and corrupting the index.

**Why this priority**: Data integrity — concurrent indexing operations on the same store can produce inconsistent state. This is a safety constraint, not a feature.

**Independent Test**: Start `dewey serve` (triggers background indexing). Immediately call the `index` MCP tool. Verify it returns "indexing operation already in progress" (or waits and succeeds after background completes).

**Acceptance Scenarios**:

1. **Given** background indexing is in progress, **When** an agent calls the `index` MCP tool, **Then** the tool returns an error indicating an operation is already in progress
2. **Given** background indexing is in progress, **When** an agent calls the `reindex` MCP tool, **Then** the tool returns the same "already in progress" error
3. **Given** background indexing has completed, **When** an agent calls `index` or `reindex`, **Then** the tools work normally (acquire the mutex, run to completion)

---

### Edge Cases

- What happens when background indexing fails (e.g., Ollama becomes unavailable mid-index)? The error is logged and the server continues operating with the previous session's data. No crash, no restart. The developer can run `/dewey-index` to retry manually.
- What happens when the developer closes OpenCode while background indexing is still running? The indexing goroutine is terminated when the process exits. Any partially-indexed content that was already persisted to SQLite remains available in the next session. Incomplete embedding batches are not persisted.
- What happens on a fresh repo with no previous index (first `dewey serve`)? The server starts with an empty in-memory index. Tools return empty results until background indexing populates the store and refreshes the in-memory index. This is acceptable — the alternative (waiting 30+ seconds) causes a timeout.
- What happens when the persistent store is not configured (in-memory mode)? Background indexing proceeds as before since there's no previous data to serve from. The server starts immediately, indexing populates the in-memory store, and results become available as indexing progresses. No behavioral change for in-memory mode.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The MCP server MUST start accepting tool calls before vault indexing begins — the server startup MUST NOT block on indexing
- **FR-002**: Vault indexing (incremental index, external page loading, embedding generation) MUST run in a background operation that starts after the MCP server is ready
- **FR-003**: During background indexing, all MCP tools that read from the persistent store (search, get_page, traverse, get_links, get_block, find_by_tag, etc.) MUST return results from the previous session's data
- **FR-004**: During background indexing, semantic search tools MUST return results from the previous session's embeddings
- **FR-005**: When background indexing completes, the in-memory index MUST be refreshed so subsequent tool calls return up-to-date results
- **FR-006**: Background indexing MUST acquire the same mutex used by the `index` and `reindex` MCP tools (spec 011) to prevent concurrent indexing operations
- **FR-007**: Background indexing start and completion MUST be logged using structured diagnostics (ISO 8601 timestamps, elapsed time, page counts) per spec 009
- **FR-008**: If background indexing encounters an error, the error MUST be logged and the server MUST continue operating with the previous session's data — no crash, no automatic restart
- **FR-009**: The file watcher MUST be started after background indexing completes, not during — watch events during indexing could cause duplicate processing
- **FR-010**: The "server ready" log line MUST appear before background indexing begins, confirming the MCP server is accepting requests

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: `dewey serve` on a repo with 255+ local pages starts accepting MCP tool calls within 2 seconds (down from 32 seconds today)
- **SC-002**: An agent can call `get_page` or `search` immediately after server startup and receive results from the persistent store
- **SC-003**: After background indexing completes, search results reflect the current state of the vault — new and modified files are included
- **SC-004**: The `index` and `reindex` MCP tools correctly reject concurrent calls during background indexing with an informative error message
- **SC-005**: All existing tests continue to pass — no behavioral regression for repos that index quickly (< 5 seconds)

## Assumptions

- The persistent store (SQLite) supports concurrent reads from MCP tool handlers while background indexing writes new pages/blocks/embeddings. SQLite's WAL (Write-Ahead Logging) mode enables this — readers see a consistent snapshot while writers append.
- The in-memory index (`vault.Client` maps) is refreshed atomically after background indexing completes. During indexing, MCP tools bypass the in-memory index and read directly from the persistent store.
- The previous session's data in the persistent store is "good enough" for the 30-60 seconds that background indexing takes. Most sessions don't start with a "search for content that was just added" — agents typically search for existing knowledge.
- The file watcher starting after indexing (not before) is acceptable. File changes during the ~30-second indexing window are not watched, but `IncrementalIndex()` will catch them because it compares file hashes against the store.
- This change affects only the `executeServe()` startup path. The CLI commands (`dewey index`, `dewey reindex`, `dewey status`, `dewey doctor`) are unchanged.
