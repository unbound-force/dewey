# Feature Specification: Documentation Sync

**Feature Branch**: `014-docs-sync`
**Created**: 2026-04-11
**Status**: Draft
**Input**: Comprehensive README and AGENTS.md update to reflect v3.0.0 features

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Accurate README for New Users (Priority: P1)

A developer discovers Dewey on GitHub and reads the README. Today it says "40 tools across 10 categories" and lists only the original graphthulhu tool categories. The developer has no idea Dewey has knowledge compilation, trust tiers, source code indexing, Ollama auto-start, background indexing, or 48 tools. The README should accurately represent what Dewey v3.0.0 can do so the developer can make an informed decision about whether to use it.

**Why this priority**: The README is the first thing new users see. Inaccurate documentation erodes trust.

**Independent Test**: Read the README and verify every feature claim matches the actual codebase. Verify tool counts, CLI commands, source types, and MCP tool names are all current.

**Acceptance Scenarios**:

1. **Given** the README, **When** a developer reads the tool count, **Then** it says "48 tools across 14 categories" (not 40/10)
2. **Given** the README, **When** a developer looks for `dewey compile`, **Then** it appears in the CLI commands section
3. **Given** the README, **When** a developer reads about semantic search tools, **Then** the tool names use the unprefixed format (`semantic_search`, not `dewey_semantic_search`)

---

### User Story 2 - Accurate AGENTS.md for AI Agents (Priority: P2)

An AI agent reads AGENTS.md to understand the project. Today it has a stale `server.go` comment ("43 tool registrations") and doesn't document the `store_learning` API change (tag/category/{tag}-{sequence} identity). The agent operates with incomplete context about the project's capabilities.

**Why this priority**: AGENTS.md is the primary context document for AI agents working on this codebase.

**Independent Test**: Grep AGENTS.md for stale references and verify all feature documentation is current.

**Acceptance Scenarios**:

1. **Given** AGENTS.md, **When** an AI reads the `server.go` comment, **Then** it says "48 tool registrations" (not 43)
2. **Given** AGENTS.md, **When** an AI looks for `store_learning` API documentation, **Then** it finds the `tag`/`category`/`{tag}-{sequence}` identity scheme documented

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: README tool count MUST be updated to 48 tools across 14 categories
- **FR-002**: README MUST document all 14 tool categories including Compile, Lint, Promote, Indexing, and Learning
- **FR-003**: README MUST use unprefixed tool names (`semantic_search`, not `dewey_semantic_search`)
- **FR-004**: README MUST document `dewey compile`, `dewey lint`, `dewey promote` CLI commands
- **FR-005**: README MUST document `store_learning`, `compile`, `lint`, `promote`, `index`, `reindex` MCP tools
- **FR-006**: README MUST document trust tiers (authored/validated/draft)
- **FR-007**: README MUST document background indexing (instant MCP startup)
- **FR-008**: README MUST document Ollama auto-start behavior
- **FR-009**: README MUST document `.gitignore` support and `ignore`/`recursive` source config
- **FR-010**: README MUST document code source type (`type: code`)
- **FR-011**: AGENTS.md `server.go` comment MUST say "48 tool registrations"
- **FR-012**: AGENTS.md MUST document the `store_learning` API change: `tag` (required), `category` (optional), `{tag}-{sequence}` identity, `created_at` timestamp
- **FR-013**: AGENTS.md MUST include a Trust Tiers section explaining authored/validated/draft

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Zero stale tool counts in README or AGENTS.md — `grep -c "40 tools\|43 tool" README.md AGENTS.md` returns 0
- **SC-002**: All 14 CLI commands appear in README — `dewey serve`, `init`, `index`, `reindex`, `status`, `search`, `source`, `doctor`, `manifest`, `journal`, `add`, `compile`, `lint`, `promote`
- **SC-003**: All existing tests continue to pass — documentation changes have no code impact

## Assumptions

- This is a documentation-only change — no production code, no tests, no schema changes.
- The README structure (sections, ordering) may be reorganized to accommodate new features. The existing content is the starting point, not a constraint.
- AGENTS.md changes are minimal (3 targeted fixes) since it was partially updated during feature implementation.
