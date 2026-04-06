# Feature Specification: Code Source Indexing & Manifest Generation

**Feature Branch**: `010-code-source-index`
**Created**: 2026-04-06
**Status**: Draft
**Input**: Source code indexing with language-aware chunking and dewey manifest generation for cross-repo API discoverability (GitHub issue #29)

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Discover Sibling Repo CLI Commands (Priority: P1)

An AI agent is planning a migration spec in the `unbound-force` repo that depends on the `replicator` CLI. Today, the agent has no way to ask Dewey "what CLI commands does replicator have?" — it must fall back to raw filesystem reads or GitHub API calls with base64 decoding. With code source indexing, the agent configures a `type: code` source pointing to `../replicator`, runs `dewey index`, and then asks Dewey via `semantic_search`. Dewey returns the relevant CLI command definitions with names, descriptions, and flag details, ranked by semantic similarity.

**Why this priority**: Cross-repo API discoverability is the primary motivator for this feature. CLI commands are the most common cross-repo query because the Unbound Force ecosystem is CLI-first.

**Independent Test**: Configure a `type: code` source pointing to a Go repo with Cobra commands. Run `dewey index`. Query `semantic_search` for "CLI commands." Verify results include command names and descriptions extracted from the source code.

**Acceptance Scenarios**:

1. **Given** a `type: code` source pointing to a Go repo with Cobra commands, **When** `dewey index` runs, **Then** CLI command definitions are indexed with their `Use`, `Short`, and `Long` fields as searchable content
2. **Given** code source content is indexed, **When** an agent queries `semantic_search` for "what commands does replicator have," **Then** results include the command definitions ranked by relevance
3. **Given** a `type: code` source with `languages: [go]`, **When** `dewey index` runs, **Then** only `.go` files are processed (not `.md`, `.json`, etc.)

---

### User Story 2 - Search Exported APIs Across Repos (Priority: P2)

An AI agent is implementing a feature that needs to call an exported function from a sibling repo's package. Today, the agent guesses at file paths or reads entire directories. With code source indexing, the agent searches for "exported types in the engine package" and Dewey returns function signatures, type definitions, and their doc comments — enough to understand the API without reading raw source files.

**Why this priority**: API discovery is the second most common cross-repo query. Knowing what types and functions a package exports is essential for cross-repo integration work.

**Independent Test**: Configure a `type: code` source pointing to a Go repo. Run `dewey index`. Query for a specific exported type name. Verify the result includes the type signature and its doc comment.

**Acceptance Scenarios**:

1. **Given** a Go repo with exported types and functions, **When** code source indexing processes the repo, **Then** each exported function signature and its doc comment are stored as a separate searchable block
2. **Given** a Go repo with package-level doc comments, **When** code source indexing processes the repo, **Then** package doc comments are indexed and searchable
3. **Given** code source indexing, **When** a function has a multi-line doc comment, **Then** the full doc comment is included in the indexed block alongside the function signature

---

### User Story 3 - Generate Project Manifest (Priority: P3)

A developer runs `dewey manifest` in their repo. Dewey introspects the source code (using the same language-aware chunker as the code source) and produces a `.dewey/manifest.md` file summarizing the project's public interface: CLI commands, MCP tools, exported packages. This manifest is a regular Markdown file that gets automatically indexed by any Dewey instance that has this repo as a disk source — no special source type needed. Other repos' agents can discover this repo's interface by searching the manifest alongside specs and documentation.

**Why this priority**: Manifests provide curated, high-signal summaries that complement the raw code indexing. They are especially useful for agents that need a quick overview rather than detailed API signatures. Lower priority because it depends on the chunker from US1/US2.

**Independent Test**: Run `dewey manifest` in a Go repo with Cobra commands and MCP tools. Verify `.dewey/manifest.md` is created with CLI Commands and MCP Tools sections. Verify it's indexable as a normal Markdown file.

**Acceptance Scenarios**:

1. **Given** a Go repo with Cobra commands, **When** the developer runs `dewey manifest`, **Then** `.dewey/manifest.md` is created with a CLI Commands section listing each command's name, description, and flags
2. **Given** a Go repo with MCP tool registrations, **When** the developer runs `dewey manifest`, **Then** the manifest includes an MCP Tools section listing each tool's name and description
3. **Given** a Go repo with exported packages, **When** the developer runs `dewey manifest`, **Then** the manifest includes an Exported Packages section with package paths and doc summaries
4. **Given** the manifest is generated, **When** another repo has this repo as a `type: disk` source, **Then** `dewey index` picks up `manifest.md` and makes it searchable

---

### User Story 4 - Pluggable Language Support (Priority: P4)

The code source indexer starts with Go support but is designed to accept additional language chunkers in the future. A `languages` field in the source configuration specifies which languages to process. When a language is not supported, the source logs a warning and skips files for that language. Adding a new language (e.g., TypeScript) requires implementing a chunker that produces blocks from source files — the indexing pipeline, storage, and search infrastructure remain unchanged.

**Why this priority**: Future-proofing the architecture. The ecosystem may expand beyond Go, and the chunker interface should be ready. This story is about the extension point, not implementing additional languages.

**Independent Test**: Configure a `type: code` source with `languages: [go, typescript]`. Verify Go files are processed and a warning is logged for the unsupported TypeScript language.

**Acceptance Scenarios**:

1. **Given** a `type: code` source with `languages: [go, typescript]`, **When** `dewey index` runs, **Then** Go files are processed and an informational message is logged indicating TypeScript is not yet supported
2. **Given** a chunker interface for language-aware parsing, **When** a new language implementation is added, **Then** no changes are required to the indexing pipeline, storage layer, or search infrastructure

---

### Edge Cases

- What happens when a source code file is very large (>10,000 lines)? The chunker processes it normally — each exported declaration becomes a separate block. The file size doesn't affect the chunker because it operates on declarations, not whole-file content.
- What happens when a Go file has syntax errors? The chunker logs a warning and skips the file. Partially parsed files are not indexed to avoid corrupted blocks.
- What happens when `dewey manifest` is run in a non-Go repo? The command reports "no supported languages detected" and produces an empty or minimal manifest with just the README content.
- What happens when the code source and disk source both point to the same repo? Both sources index their respective file types. The disk source indexes `.md` files, the code source indexes `.go` files. No duplication because they have different source IDs and file type filters.
- What happens when `.gitignore` excludes source files? Code sources respect `.gitignore` the same way disk sources do (spec 006). `vendor/`, `testdata/`, and other gitignored directories are automatically skipped.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: Dewey MUST support a `type: code` source in `sources.yaml` that indexes source code files with language-aware chunking
- **FR-002**: The `type: code` source MUST accept a `languages` configuration field (list of language identifiers) specifying which languages to process
- **FR-003**: The Go language chunker MUST extract package doc comments, exported function/method signatures with doc comments, exported type definitions with doc comments, and exported constant/variable declarations
- **FR-004**: The Go language chunker MUST extract Cobra CLI command definitions (`Use`, `Short`, `Long` fields) and MCP tool registrations (`Name`, `Description` fields) as high-signal searchable blocks
- **FR-005**: The Go language chunker MUST skip function bodies — only signatures and doc comments are indexed, not implementation details
- **FR-006**: Each extracted declaration MUST be stored as a separate block in the index, enabling block-level search and embedding granularity
- **FR-007**: The `type: code` source MUST support the same `ignore`, `recursive`, and `.gitignore` respect as disk sources (spec 006 infrastructure)
- **FR-008**: The `type: code` source MUST support `include` and `exclude` path pattern lists for filtering which directories and files are indexed
- **FR-009**: When a configured language has no available chunker, the system MUST log a warning and skip files for that language without failing the index operation
- **FR-010**: Dewey MUST provide a `dewey manifest` CLI command that introspects the current repo's source code and generates a `.dewey/manifest.md` file summarizing CLI commands, MCP tools, and exported packages
- **FR-011**: The `dewey manifest` command MUST use the same language-aware chunker infrastructure as the code source type
- **FR-012**: The generated `manifest.md` MUST be a standard Markdown file indexable by any Dewey disk source without special handling
- **FR-013**: Source code files with syntax errors MUST be skipped with a logged warning — the index operation MUST NOT fail due to individual file parse errors
- **FR-014**: Test files (matching `*_test.go` or equivalent per-language conventions) MUST be excluded from indexing by default

### Key Entities

- **Code Source**: A content source type (`type: code`) that indexes source code files using language-aware chunking. Configured in `sources.yaml` with `languages`, `include`, `exclude`, `ignore`, and `recursive` fields.
- **Language Chunker**: A pluggable component that parses source files for a specific language and extracts high-signal blocks (declarations, signatures, doc comments). Go is the initial implementation. Each chunker produces blocks compatible with the existing store pipeline.
- **Manifest**: A Markdown file (`.dewey/manifest.md`) generated by `dewey manifest` that summarizes a project's public interface. Consumed by disk sources in other repos as a searchable document.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: An agent can discover CLI commands from a sibling Go repo by querying `semantic_search` — results include command name and description within the top 5 results for relevant queries
- **SC-002**: An agent can discover exported types and functions from a sibling Go repo by querying `semantic_search` — results include the function signature and doc comment
- **SC-003**: `dewey manifest` generates a `.dewey/manifest.md` file that contains all CLI commands, MCP tools, and exported packages found in the current repo
- **SC-004**: The code source indexes a typical Go repo (100 files, 10,000 lines) in under 10 seconds excluding embedding generation
- **SC-005**: All existing tests continue to pass — code source indexing does not affect disk source, vault, or MCP tool behavior
- **SC-006**: Adding a new language chunker requires no changes to the indexing pipeline, storage layer, or search infrastructure — only a new chunker implementation and registration

## Assumptions

- The Go language chunker uses the standard library's `go/parser` and `go/ast` packages for source code parsing. This provides correct handling of multi-line strings, doc comment association, and type-safe AST walking without external dependencies.
- Cobra command definitions follow the standard pattern of struct literals with `Use`, `Short`, and `Long` string fields. Non-standard patterns (e.g., dynamically generated commands) may not be detected.
- MCP tool registrations follow the pattern used in this project: `mcp.AddTool(srv, &mcp.Tool{Name: "...", Description: "..."}, handler)`. Other MCP SDK patterns may not be detected initially.
- The `dewey manifest` command is run in the source repo by the developer or CI, not by Dewey itself during indexing. The manifest is a pre-generated artifact, not a live query.
- The `include` and `exclude` fields use glob patterns (matching `.gitignore` syntax from spec 006) for consistency with the existing ignore infrastructure.
- Embedding quality for code snippets (function signatures + doc comments) with `granite-embedding:30m` is assumed to be adequate. Natural language doc comments embed well; raw Go syntax may embed less accurately. The doc-comment-first approach mitigates this.
