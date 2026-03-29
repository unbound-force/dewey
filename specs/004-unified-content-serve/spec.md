# Feature Specification: Unified Content Serve

**Feature Branch**: `004-unified-content-serve`
**Created**: 2026-03-28
**Status**: Complete
**Input**: User description: "Unified content serve: teach dewey serve to load and query external-source content (GitHub issues, web crawl, cross-repo docs) from graph.db alongside local vault files, making all indexed content queryable via MCP tools"

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Search Across All Indexed Sources (Priority: P1)

A developer uses `dewey index` to fetch content from multiple sources: local Markdown files, GitHub issues/PRs from their organization, and web-crawled documentation. When they start `dewey serve` and an AI agent uses MCP tools to search for information, the results include matches from all sources -- not just local files.

Today, `dewey index` fetches documents from external sources and stores metadata in `graph.db`, but `dewey serve` only queries local Markdown files. External content is invisible to MCP tools and CLI search. This story closes that gap: all indexed content becomes searchable through a single unified interface.

**Why this priority**: This is the core value proposition. Without it, `dewey index` and content sources are effectively non-functional features -- they fetch and store data that nothing can query.

**Independent Test**: Can be fully tested by running `dewey index` with a GitHub source, then starting `dewey serve` and using the `dewey_search` MCP tool. Results MUST include matches from GitHub-sourced content.

**Acceptance Scenarios**:

1. **Given** a user has configured a GitHub source in `sources.yaml` and run `dewey index`, **When** they start `dewey serve` and an agent calls the `dewey_search` MCP tool with a query matching a GitHub issue title, **Then** the search results include that GitHub issue with its source identified.
2. **Given** a user has indexed web-crawled documentation via `dewey index`, **When** they query via the `dewey_full_text_search` MCP tool, **Then** results include matches from web-crawled content alongside local file matches.
3. **Given** a user has indexed content from 3 sources (disk, GitHub, web), **When** they call `dewey_get_all_pages` via MCP, **Then** all pages from all sources appear in the listing, each with its source identifiable.

---

### User Story 2 - Full Content Persistence During Indexing (Priority: P1)

When a developer runs `dewey index`, the system parses fetched documents into structured blocks and links (not just page metadata). This means the content is immediately available for block-level search, backlink discovery, and semantic search -- without needing to re-fetch from the original source.

Today, `dewey index` stores only page-level metadata (name, hash, source ID). The actual document content is discarded after hashing. This story ensures the full content pipeline runs during indexing: parse into blocks, extract links, and generate embeddings.

**Why this priority**: This is a prerequisite for Story 1. External content cannot be served if it was never fully persisted. Equally critical because without this, GitHub content is irrecoverable after indexing without re-fetching.

**Independent Test**: Can be fully tested by running `dewey index` with a GitHub source, then querying `graph.db` directly to verify blocks, links, and embeddings exist for the indexed GitHub pages.

**Acceptance Scenarios**:

1. **Given** a GitHub source is configured with issues containing `[[wikilink]]` syntax, **When** the user runs `dewey index`, **Then** the store contains block records for each issue's content sections AND link records for each extracted wikilink.
2. **Given** Ollama is running with an embedding model, **When** `dewey index` completes, **Then** embeddings exist in the store for blocks from all sources (not just disk sources).
3. **Given** `dewey index` has previously indexed a GitHub issue, **When** the issue content changes on GitHub and the user re-runs `dewey index`, **Then** the stored blocks and links are updated to reflect the new content.

---

### User Story 3 - Cross-Source Backlinks (Priority: P2)

When a GitHub issue references a local page name using `[[page name]]` wikilink syntax, that reference appears in the local page's backlinks. This enables AI agents to discover relationships between external and local content -- for example, finding all GitHub issues that reference a specific design document.

**Why this priority**: Backlinks are a key differentiator of a knowledge graph over flat search. Cross-source backlinks emerge naturally once external content is loaded into the vault's in-memory index, but this story ensures the behavior is intentional and tested.

**Independent Test**: Can be fully tested by creating a local page `design-doc.md`, a GitHub issue body containing `[[design-doc]]`, running `dewey index` then `dewey serve`, and calling the `dewey_get_page_linked_references` MCP tool on `design-doc`.

**Acceptance Scenarios**:

1. **Given** a local page named `architecture` exists and a GitHub issue body contains `[[architecture]]`, **When** an agent calls `dewey_get_page_linked_references` for the `architecture` page, **Then** the backlinks include the GitHub issue as a referring page.
2. **Given** two GitHub issues reference each other via wikilinks, **When** an agent queries backlinks for either issue, **Then** the cross-reference appears in the backlinks.

---

### User Story 4 - Read-Only Protection for External Content (Priority: P2)

When an AI agent attempts to edit content that originated from an external source (GitHub, web crawl), the system returns a clear error explaining that the content is read-only and originates from an external source. This prevents data integrity issues where local edits would diverge from the authoritative upstream source.

Content created directly through MCP tools (e.g., `dewey_create_page`) remains fully editable.

**Why this priority**: Without write guards, MCP write operations on external pages would either fail with cryptic errors (no local `.md` file exists) or corrupt the in-memory state. Clear error messages guide agents to the correct behavior.

**Independent Test**: Can be fully tested by loading an external-source page, then calling `dewey_update_block` on one of its blocks and verifying the error response.

**Acceptance Scenarios**:

1. **Given** a page originated from a GitHub source, **When** an agent calls `dewey_update_block` on one of its blocks, **Then** the tool returns an error message stating the page is read-only and identifying the source.
2. **Given** a page originated from a GitHub source, **When** an agent calls `dewey_delete_page` on it, **Then** the tool returns a read-only error.
3. **Given** a page was created locally via `dewey_create_page`, **When** an agent calls `dewey_update_block`, **Then** the update succeeds normally.

---

### User Story 5 - Semantic Search Across All Sources (Priority: P3)

When a developer has Ollama running and embedding models available, `dewey index` generates vector embeddings for external-source content. The `dewey_semantic_search` MCP tool returns semantically similar results from all sources -- not just local files.

**Why this priority**: Semantic search is an optional enhancement (requires Ollama). The core search and backlink stories deliver value without it. But when available, semantic search across all sources is a powerful capability.

**Independent Test**: Can be fully tested by indexing GitHub content with Ollama running, then calling `dewey_semantic_search` with a conceptually related query and verifying external-source results appear.

**Acceptance Scenarios**:

1. **Given** Ollama is running and `dewey index` has indexed GitHub issues with embeddings, **When** an agent calls `dewey_semantic_search` with a query semantically related to a GitHub issue, **Then** the issue appears in the results with a similarity score.
2. **Given** Ollama is NOT running during `dewey index`, **When** the index completes, **Then** page metadata and blocks are still persisted (embeddings are skipped gracefully) and keyword search still works for external content.

---

### Edge Cases

- What happens when a GitHub issue title collides with a local page name? The system MUST handle name collisions by prefixing external pages with their source path (e.g., `github/org/repo/issues/42`) to avoid overwriting local pages.
- What happens when `dewey index` is run while `dewey serve` is already running? The two processes share `graph.db` via SQLite WAL mode, which allows concurrent reads and writes without blocking. The serve process does not hot-reload external content; the user must restart `dewey serve` to pick up newly indexed external content.
- What happens when a previously indexed external source is removed from `sources.yaml` and `dewey index` is re-run? `dewey index` auto-purges pages, blocks, links, and embeddings belonging to source IDs no longer present in `sources.yaml`. The system MUST log the purge action with the count of removed pages.
- How does the system handle very large external content volumes (e.g., 10,000 GitHub issues)? The system loads all external pages into memory on startup. For volumes exceeding 5,000 pages, the system SHOULD log a warning about memory usage.
- What happens when web-crawled content contains no markdown structure (no headings)? The entire document becomes a single block. It is still searchable and embeddable, but backlink extraction will only find wikilinks if the plain text happens to contain `[[link]]` syntax.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: `dewey index` MUST parse fetched documents into blocks, preserving heading-based hierarchy for markdown content and treating unstructured text as a single block.
- **FR-002**: `dewey index` MUST extract wikilinks (`[[page name]]`) from block content and persist them as link records in the store.
- **FR-003**: `dewey index` MUST generate vector embeddings for external-source blocks when an embedding provider is available, and skip gracefully when unavailable.
- **FR-004**: `dewey index` MUST update blocks, links, and embeddings when re-indexing a document whose content hash has changed.
- **FR-005**: `dewey serve` MUST load pages from the store that originated from non-local sources (GitHub, web crawl) into its in-memory index on startup.
- **FR-006**: All 40 MCP tools MUST return results that include external-source pages where applicable (search, list, backlinks, semantic search).
- **FR-007**: The system MUST prefix external page names with a source-derived namespace (e.g., `github/org/repo/issues/42`) to prevent name collisions with local pages.
- **FR-008**: MCP write operations (create, update, delete, move, rename) MUST reject modifications to pages originating from external sources with a clear error message identifying the source.
- **FR-009**: MCP write operations MUST continue to work normally for pages originating from the local disk source.
- **FR-010**: `dewey status` MUST report the count of pages by source, distinguishing local vault pages from each external source.
- **FR-011**: The existing 37 inherited graphthulhu MCP tools MUST continue to produce identical results for local vault content after this change.
- **FR-012**: The store MUST open `graph.db` in WAL (Write-Ahead Logging) journal mode to allow concurrent read/write access from separate `dewey serve` and `dewey index` processes without blocking or `SQLITE_BUSY` errors.
- **FR-013**: `dewey index` MUST auto-purge pages, blocks, links, and embeddings for source IDs that are no longer present in `sources.yaml`, and MUST log the count of purged pages.
- **FR-014**: Both `dewey index` and `dewey serve` MUST emit structured log lines at phase boundaries (start/completion of external page loading, block parsing, link extraction, embedding generation) including counts and elapsed time.

### Key Entities

- **External Page**: A page in the knowledge graph that originated from a non-local source (GitHub API, web crawl). Has a source identifier, is loaded from the store (not from a local `.md` file), and is read-only via MCP tools.
- **Block**: A content unit within a page (typically a section under a heading). Blocks are the granularity for search, backlinks, and embeddings. External pages now have blocks persisted by `dewey index`.
- **Link**: A directed reference from one page to another, extracted from `[[wikilink]]` syntax in block content. Links can now span across source boundaries (e.g., a GitHub issue linking to a local page).
- **Source Record**: Metadata about a content source (type, name, last fetch time, status). Used to distinguish local from external pages and to track indexing state.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: All content indexed by `dewey index` (from any configured source) is queryable via MCP search tools within 5 seconds of `dewey serve` startup.
- **SC-002**: Cross-source backlinks are discovered and returned by the backlinks MCP tool with the same response structure as local backlinks.
- **SC-003**: Write operations on external-source pages return a descriptive error within 100 milliseconds, without corrupting in-memory state.
- **SC-004**: `dewey status` reports accurate page counts per source, matching the number of documents fetched by `dewey index`.
- **SC-005**: The existing 37 graphthulhu-compatible MCP tools produce identical results for local vault content before and after this change (backward compatibility).
- **SC-006**: Startup time for `dewey serve` increases by no more than 2 seconds when loading up to 1,000 external pages from the store.

## Clarifications

### Session 2026-03-28

- Q: What should happen when `dewey index` writes to `graph.db` while `dewey serve` is running? → A: Enable SQLite WAL (Write-Ahead Logging) journal mode so reads and writes can proceed concurrently without blocking.
- Q: How should orphaned external pages be handled when their source is removed from configuration? → A: Auto-purge — `dewey index` deletes pages, blocks, links, and embeddings for sources no longer in `sources.yaml`.
- Q: What level of observability should the system provide for external content operations? → A: Structured checkpoints — log start/completion of each phase (loading, parsing, embedding) with counts and timing.

## Assumptions

- External-source content is treated as reference material and is read-only via MCP tools. The authoritative copy lives at the source (GitHub, web).
- Name collisions between external and local pages are resolved by namespacing external pages with their source path, not by overwriting local pages.
- The `dewey serve` process does not hot-reload content indexed by a concurrent `dewey index` run. A restart is required to pick up new external content.
- Web-crawled content (plain text from HTML stripping) has limited structural parsing (no heading-based block decomposition) but is still searchable as a single block.
- Embedding generation during `dewey index` follows the same graceful-degradation pattern as `dewey serve`: available when Ollama is running, skipped otherwise.

## Dependencies

- `dewey index` and content sources (`source/` package) must be functional and tested (completed in spec 001).
- The store schema (`store/migrate.go`) must support blocks, links, and embeddings for external-source pages (existing schema is sufficient; no new tables needed).
- Ollama must be available locally for embedding generation (optional; the feature degrades gracefully without it).
