# Feature Specification: Dewey Core Implementation

**Feature Branch**: `001-core-implementation`
**Created**: 2026-03-22
**Status**: Complete
**Input**: Dewey design paper (`dewey-design-paper.md`) and orchestration plan (`dewey-orchestration-plan.md`), Phase 2

## User Scenarios & Testing *(mandatory)*

### User Story 1 -- Persistent Knowledge Graph Index (Priority: P1)

A developer starts an OpenCode session in a repository that contains 200+ Markdown files. Today, Dewey (inherited from graphthulhu) rebuilds its entire in-memory knowledge graph from scratch on every session start, taking 1-3 seconds. When the developer closes their session and opens a new one minutes later, the exact same index is rebuilt from zero.

With persistent storage, Dewey saves its knowledge graph index to disk after indexing. On the next session start, Dewey loads the persisted index and only processes files that changed since the last indexing run. The developer experiences near-instant startup for subsequent sessions. All existing knowledge graph tools (the 37 MCP tools inherited from graphthulhu) continue to work identically -- the persistence is transparent to tool callers.

**Why this priority**: Persistence is the foundation for everything else. Vector embeddings (US2) need persistent storage. Content sources (US3) need persistent caching. Without persistence, Dewey is functionally identical to graphthulhu with a different name. This is the minimum viable differentiation.

**Independent Test**: Start Dewey on a test vault, verify the index is persisted to disk. Stop Dewey, modify one file, restart, and confirm only the modified file is re-indexed while all other pages are loaded from the persisted index. Run the full graphthulhu test suite to confirm all 37 MCP tools produce identical results.

**Acceptance Scenarios**:

1. **Given** a repository with 200 Markdown files and no existing Dewey index, **When** `dewey serve` is started for the first time, **Then** Dewey indexes all files, persists the index to `.dewey/`, and serves queries normally.
2. **Given** a persisted index from a previous session, **When** `dewey serve` starts and 3 files have changed since the last session, **Then** Dewey loads the persisted index, re-indexes only the 3 changed files, and is ready to serve queries within 2 seconds.
3. **Given** a persisted index, **When** an MCP client calls any of the 37 existing tools (e.g., `get_page`, `search`, `traverse`), **Then** the tool returns identical results to what graphthulhu would return for the same query on the same content.
4. **Given** Dewey is running with a persisted index, **When** a file is modified on disk while the server is active, **Then** the file system watcher detects the change, re-indexes the file, and updates both the in-memory and persisted indexes.

---

### User Story 2 -- Semantic Search via Vector Embeddings (Priority: P2)

A developer (or an AI agent persona) needs to find content that is conceptually related to a query but uses different terminology. For example, searching for "authentication timeout" should find a document titled "login session expiry" even though the words do not overlap. Today's keyword-based search misses these conceptual matches entirely.

With vector search, Dewey generates embeddings for all indexed content using a locally-run embedding model (via Ollama). When a user issues a semantic search query, Dewey converts the query to an embedding and finds the most similar documents by vector similarity. Results include provenance metadata (source, similarity score, fetch timestamp) so the caller can assess quality and attribution.

**Why this priority**: Semantic search is the primary new capability that distinguishes Dewey from graphthulhu. It requires persistence (US1) to store embeddings. It unlocks the ability for agent personas to discover relevant context they didn't know to ask for -- the core value proposition described in the design paper.

**Independent Test**: Index a test vault with known content. Issue a semantic search query using different terminology than the indexed content. Verify that conceptually related documents are returned with similarity scores above a relevance threshold. Verify that keyword searches for the same query do not find those documents (proving the semantic search adds value beyond keyword matching).

**Acceptance Scenarios**:

1. **Given** a repository with indexed content including a page about "login session expiry," **When** a user searches semantically for "authentication timeout," **Then** the login session expiry page appears in the results with a similarity score.
2. **Given** indexed content, **When** a user issues a semantic search, **Then** each result includes provenance metadata: document source, similarity score, and the timestamp when the content was last indexed.
3. **Given** a specific document in the index, **When** a user requests similar documents, **Then** Dewey returns the most similar documents ranked by vector similarity.
4. **Given** indexed content from multiple sources, **When** a user issues a semantic search with a source filter (e.g., only local files, or only a specific repository), **Then** results are restricted to the specified source.
5. **Given** a file is added or modified in the repository, **When** the file is re-indexed, **Then** its embeddings are generated (or regenerated) and persisted alongside the knowledge graph index.
6. **Given** Ollama is not running or the embedding model is not installed, **When** `dewey serve` starts, **Then** Dewey starts successfully with all existing keyword-based tools working, but semantic search tools return a clear error message indicating the embedding model is unavailable.

---

### User Story 3 -- Pluggable Content Sources (Priority: P3)

A developer working on the Dewey repository needs context from across the Unbound Force organization -- open GitHub issues in the Gaze repo, API documentation from the Go standard library, or specifications from the meta repo. Today, Dewey only indexes local Markdown files in the current repository. Cross-repository and external documentation context is unavailable.

With pluggable content sources, the developer configures additional sources in a YAML file: GitHub repositories to watch for issues and pull requests, documentation URLs to crawl. Dewey fetches content from these sources, indexes it alongside local files, and makes it searchable through the same MCP tools. Each source has a configurable refresh interval. Source content is cached locally so it does not need to be re-fetched on every session start.

**Why this priority**: Content sources depend on both persistence (US1) for caching and optionally on embeddings (US2) for semantic search across external content. This is the highest-value capability for the autonomous define workflow (Phase 5 of the orchestration plan) but also the most complex. It can be delivered after US1 and US2 are complete.

**Independent Test**: Configure a GitHub source pointing to a test repository with known issues. Run `dewey index`. Verify the issues appear in search results with correct provenance metadata (source: github, repo name, issue URL). Verify that subsequent `dewey index` runs only fetch issues updated since the last fetch.

**Acceptance Scenarios**:

1. **Given** a source configuration file listing a GitHub organization and specific repositories, **When** `dewey index` is run, **Then** Dewey fetches open and recently closed issues, pull request descriptions, and repository READMEs from those repositories and indexes them.
2. **Given** a source configuration listing documentation URLs, **When** `dewey index` is run, **Then** Dewey crawls those URLs (respecting robots.txt and rate limits), converts HTML to plain text, and indexes the content.
3. **Given** previously fetched source content cached locally, **When** `dewey index` is run again, **Then** only content updated since the last fetch is re-fetched (incremental update based on timestamps).
4. **Given** a source with `refresh: daily` configured, **When** less than 24 hours have passed since the last fetch, **Then** `dewey index` skips that source and uses cached content.
5. **Given** no source configuration file exists, **When** `dewey serve` starts, **Then** Dewey indexes only local Markdown files (equivalent to current graphthulhu behavior) with no errors.
6. **Given** a GitHub source is configured but network access is unavailable, **When** `dewey index` is run, **Then** Dewey reports the fetch failure clearly and continues serving from cached content (if any) without crashing.

---

### User Story 4 -- CLI Commands and Configuration (Priority: P4)

A developer installing Dewey for the first time needs to initialize it in their repository, configure content sources, build the initial index, and check index health. Today, Dewey has only the `serve` command (inherited from graphthulhu) and CLI subcommands for journal/add/search. There is no way to initialize a Dewey configuration, trigger an index build, or inspect index status.

With the new CLI commands, the developer runs `dewey init` to create the `.dewey/` directory with default configuration. They run `dewey source add` to configure external sources interactively. They run `dewey index` to build the initial index (including fetching external sources). They run `dewey status` to inspect index health: how many pages are indexed, which sources are configured, when each was last refreshed, and whether the embedding model is available.

**Why this priority**: CLI commands are the user-facing interface for managing the features in US1-US3. They can be developed incrementally as each underlying capability lands. The `init` and `status` commands can ship with US1; `index` and `source add` ship with US3.

**Independent Test**: Run `dewey init` in an empty directory. Verify `.dewey/` is created with a default configuration file. Run `dewey status` and verify it reports index statistics (0 pages if freshly initialized). Run `dewey index` and verify the index is built from local files.

**Acceptance Scenarios**:

1. **Given** a repository with no `.dewey/` directory, **When** `dewey init` is run, **Then** Dewey creates `.dewey/` with a default configuration file specifying the local disk as the only source and the default embedding model.
2. **Given** an initialized Dewey configuration, **When** `dewey status` is run, **Then** it reports: total pages indexed, source count and freshness, embedding coverage (percentage of pages with embeddings), and embedding model availability.
3. **Given** an initialized Dewey configuration, **When** `dewey index` is run, **Then** Dewey indexes all local Markdown files and fetches any configured external sources.
4. **Given** a running Dewey instance, **When** `dewey status` is queried via the `health` MCP tool, **Then** the response includes the same information as the CLI `status` command in a structured format.

---

### Edge Cases

- What happens when the persisted index is corrupted or in an incompatible format? Dewey MUST detect the corruption, discard the invalid index, and perform a full re-index from scratch with a clear warning message.
- What happens when disk space is insufficient to persist the index? Dewey MUST report a clear error and continue operating from the in-memory index without crashing.
- What happens when a configured embedding model is removed from Ollama between sessions? Dewey MUST start successfully with keyword-based tools working, report the missing model in `dewey status`, and return clear error messages from semantic search tools.
- What happens when a GitHub API rate limit is exceeded during `dewey index`? Dewey MUST report the rate limit, stop fetching from that source, and continue with other sources and cached content.
- What happens when a web crawl target returns non-HTML content (PDF, binary)? Dewey MUST skip non-indexable content with a warning and continue crawling other URLs.
- What happens when two Dewey processes attempt to write to the same `.dewey/` directory simultaneously? Dewey MUST use file-level locking to prevent index corruption.
- What happens when the configured embedding model changes between sessions (e.g., switching from `granite-embedding:30m` to `granite-embedding:278m`)? Dewey MUST detect the model change, invalidate all existing embeddings, and regenerate them on the next `dewey index` run. `dewey status` MUST report the mismatch.

## Clarifications

### Session 2026-03-22

- Q: Convention pack conflicts (cobra, charmbracelet/log, internal/ layout) -- override or refactor? → A: Refactor CLI to cobra and add charmbracelet/log. The existing `flag`-based CLI and `fmt.Fprintf` logging will be migrated as part of this feature.
- Q: GitHub API credential handling -- how is the token obtained and protected? → A: Token precedence: `GITHUB_TOKEN` or `GH_TOKEN` env var → `gh auth token` subprocess → unauthenticated access with rate limit warning. Tokens MUST NOT be logged, persisted to `.dewey/`, or stored in any plaintext file. Required scope: read-only. Inspired by secure credential storage patterns from gcal-organizer spec 007.
- Q: Web crawl URL validation and safety constraints? → A: Restrict to `http://` and `https://` schemes only. Max response body 1MB. Same-domain-only by default. Max 100 pages per source. Follow redirects within same domain only.
- Q: Homebrew distribution path for SC-005? → A: Add a task to configure the Homebrew tap (GoReleaser brews section + formula in unbound-force/homebrew-tap).
- Q: Schema migration strategy for upgrades? → A: Initial schema version 1. Automatic forward migration on startup. Fallback to full re-index if migration fails.
- Q: Windows support in GoReleaser? → A: Remove Windows from GoReleaser. Target platforms are macOS and Linux only.

## Requirements *(mandatory)*

### Functional Requirements

**Persistence (US1)**:
- **FR-001**: Dewey MUST persist the knowledge graph index (pages, blocks, links, properties) to the `.dewey/` directory so the index survives process restarts.
- **FR-002**: Dewey MUST detect files that changed since the last indexing run and re-index only those files on startup (incremental update).
- **FR-003**: All 37 existing MCP tools MUST produce identical results whether the index was loaded from persistence or built from scratch.
- **FR-004**: The file system watcher MUST update both the in-memory and persisted indexes when files change while the server is running.
- **FR-005**: Dewey MUST handle corrupted or incompatible persisted indexes by discarding them and performing a full re-index with a warning.

**Vector Search (US2)**:
- **FR-006**: Dewey MUST generate vector embeddings for all indexed content using a locally-run embedding model accessed through a standard inference API.
- **FR-007**: Dewey MUST persist embeddings alongside the knowledge graph index so they survive process restarts.
- **FR-008**: Dewey MUST expose a semantic search MCP tool that accepts a natural language query and returns documents ranked by vector similarity.
- **FR-009**: Dewey MUST expose a similarity MCP tool that accepts a document identifier and returns the most similar documents.
- **FR-010**: Dewey MUST expose a filtered semantic search MCP tool that combines vector similarity with metadata filters (source type, repository, property values).
- **FR-011**: Every search result MUST include provenance metadata: source type, document origin, similarity score, and the timestamp when the content was last indexed.
- **FR-012**: The embedding model MUST be configurable via a configuration file. The default model MUST be specified in the configuration.
- **FR-013**: Dewey MUST start and serve keyword-based queries successfully even when the embedding model is unavailable. Semantic search tools MUST return a clear error indicating the model is not available.

**Content Sources (US3)**:
- **FR-014**: Dewey MUST support a pluggable source architecture where each source type implements a common contract for listing, fetching, diffing, and describing content.
- **FR-015**: Dewey MUST include a GitHub source that fetches issues, pull request descriptions, repository READMEs, and documentation files from configurable repositories.
- **FR-015a**: The GitHub source MUST obtain authentication tokens using this precedence: (1) `GITHUB_TOKEN` or `GH_TOKEN` environment variable, (2) `gh auth token` subprocess if `gh` CLI is available, (3) unauthenticated access with a rate limit warning. Tokens MUST NOT be logged, persisted to `.dewey/`, or stored in any plaintext file.
- **FR-015b**: When `gh` CLI is not installed and no environment variable token is set, the GitHub source MUST operate in unauthenticated mode (60 requests/hour rate limit) and log a warning suggesting authentication for higher limits.
- **FR-016**: Dewey MUST include a web crawl source that fetches and indexes documentation from configurable URLs, converting HTML to plain text.
- **FR-017**: The web crawl source MUST respect `robots.txt` directives and enforce configurable rate limits between requests.
- **FR-017a**: The web crawl source MUST restrict URLs to `http://` and `https://` schemes only. Other schemes (e.g., `file://`, `ftp://`) MUST be rejected.
- **FR-017b**: The web crawl source MUST enforce a maximum response body size of 1MB per page and a maximum of 100 pages per configured source.
- **FR-017c**: The web crawl source MUST follow redirects only within the same domain as the configured URL. Cross-domain redirects MUST be skipped with a warning.
- **FR-018**: Source content MUST be cached locally so it does not need to be re-fetched on every session start.
- **FR-019**: Each source MUST support a configurable refresh interval. Cached content within the interval MUST be used without re-fetching.
- **FR-020**: Source failures (network errors, rate limits, authentication failures) MUST be reported clearly without crashing. Dewey MUST continue serving from cached content when available.

**CLI and Configuration (US4)**:
- **FR-021**: `dewey init` MUST create the `.dewey/` directory with a default configuration file.
- **FR-022**: `dewey index` MUST trigger indexing of all configured sources (local files + external sources).
- **FR-023**: `dewey status` MUST report: total indexed pages, configured sources with last refresh timestamps, embedding coverage, and embedding model availability.
- **FR-024**: Source configuration MUST be defined in a YAML file within the `.dewey/` directory.
- **FR-025**: `dewey serve` MUST remain backward compatible -- an MCP client configured for the current Dewey (or graphthulhu) MUST work without configuration changes.
- **FR-026**: The CLI MUST be refactored from the current `flag.FlagSet` routing to use `github.com/spf13/cobra` for command routing and flag parsing, aligning with the Go convention pack (CS-009).
- **FR-027**: All application logging MUST use `github.com/charmbracelet/log` instead of `fmt.Fprintf(os.Stderr, ...)`, aligning with the Go convention pack (CS-008).
- **FR-028**: All store operations MUST use parameterized queries (prepared statements) to prevent SQL injection from user-derived content (page names, issue titles, crawled page titles).

### Key Entities

- **Page**: A document in the knowledge graph. Has a name, properties (from frontmatter), content blocks, and links to other pages. Persisted with a content hash for change detection.
- **Block**: A section within a page (heading-delimited). Has a UUID, content text, parent block reference, and child blocks. Forms a hierarchical tree within a page.
- **Embedding**: A vector representation of a content chunk. Associated with a specific block or page, a model identifier, and a generation timestamp. Used for similarity comparisons.
- **Source**: A configured origin for content. Has a type (disk, github, web), configuration parameters, a refresh interval, and a last-fetched timestamp. Produces documents for indexing.
- **Document**: A unit of content from any source. Has an identifier, source attribution, raw content, parsed metadata, and optionally an embedding. Normalized to a common format regardless of source.

### Assumptions

- The embedding model inference service (Ollama) is separately installed and managed by the developer. Dewey does not install or manage embedding models.
- The default embedding model (`granite-embedding:30m`, 63 MB) is sufficient for English-language code repositories. Multilingual support is available via the larger model variant but is not the default.
- GitHub API access uses a token precedence chain: `GITHUB_TOKEN`/`GH_TOKEN` env var → `gh auth token` subprocess → unauthenticated. Dewey does not manage, store, or persist GitHub credentials. Tokens are obtained at runtime only and never written to disk or logs.
- The `.dewey/` directory is added to `.gitignore` by default since it contains local index state that should not be committed to the repository.
- All data processing happens locally on the developer's machine. No content is sent to external services for embedding generation or indexing.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Subsequent session startup (with a persisted index and fewer than 10 changed files) completes in under 2 seconds for a repository with 200 Markdown files.
- **SC-002**: A semantic search query for a concept returns relevant results that a keyword search for the same concept does not find, demonstrating the value of vector embeddings over text matching.
- **SC-003**: All 37 existing MCP tools produce identical results after the implementation as they did before (backward compatibility verified by the existing test suite).
- **SC-004**: Content from at least two distinct source types (e.g., local files + GitHub issues) appears in unified search results with correct provenance metadata.
- **SC-005**: A developer can go from `brew install dewey` to serving queries with `dewey serve` in under 5 minutes, following the documented setup workflow.
- **SC-006**: The system functions with reduced capability (keyword search only, no semantic search) when the embedding model is unavailable, with no crashes or silent failures.
- **SC-007**: External source content is available immediately on session start from cache, without re-fetching, when the refresh interval has not expired.
