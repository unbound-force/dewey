# Feature Specification: Website Documentation for Dewey

**Feature Branch**: `003-website-docs`
**Created**: 2026-03-26
**Status**: Draft
**Input**: User description: "documentation (Phase 4.4)"

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Developer discovers Dewey and installs it (Priority: P1)

A developer new to the Unbound Force ecosystem visits the website and wants to understand what Dewey does, how to install it, and how to start using it in their repository. They navigate to the getting-started knowledge guide, follow the installation steps, and configure Dewey for their project within 10 minutes.

**Why this priority**: Without clear onboarding documentation, developers cannot adopt Dewey. This is the critical path to all other usage.

**Independent Test**: A developer unfamiliar with Dewey can follow the getting-started knowledge guide from zero to a working `dewey serve` session with semantic search enabled, without consulting any other documentation.

**Acceptance Scenarios**:

1. **Given** a developer visits the getting-started knowledge guide, **When** they follow the installation steps, **Then** they have Dewey installed via Homebrew with Ollama and the embedding model pulled.
2. **Given** Dewey is installed, **When** the developer runs `dewey init` in their repository, **Then** they have a `.dewey/` directory with configuration ready for customization.
3. **Given** Dewey is initialized, **When** the developer runs `dewey serve`, **Then** the MCP server starts and is available to their AI coding environment.
4. **Given** the developer wants cross-repository context, **When** they follow the source configuration section, **Then** they can add GitHub API and web crawl sources and run `dewey index` to populate them.

---

### User Story 2 - Role-specific user understands how Dewey helps their workflow (Priority: P1)

An existing Unbound Force user in a specific role (developer, tester, product owner, product manager) visits their role-specific guide and understands how Dewey enhances their workflows with semantic knowledge retrieval. They learn which Dewey tools are most relevant to their role and see concrete query examples.

**Why this priority**: Role-specific guidance turns a generic tool into actionable workflow improvement. Each role interacts with Dewey differently and needs targeted examples.

**Independent Test**: A user reading any single role guide (developer, tester, product-owner, product-manager) finds a Dewey subsection that explains 2-3 specific query patterns relevant to their role with concrete examples.

**Acceptance Scenarios**:

1. **Given** a developer reads the developer guide, **When** they look for knowledge retrieval guidance, **Then** they find examples of semantic search for API documentation and implementation patterns from other repositories.
2. **Given** a tester reads the tester guide, **When** they look for knowledge retrieval guidance, **Then** they find examples of discovering test patterns, quality baselines, and known failure modes.
3. **Given** a product owner reads the product-owner guide, **When** they look for knowledge retrieval guidance, **Then** they find examples of searching GitHub issues across the organization and finding past acceptance criteria.
4. **Given** a product manager reads the product-manager guide, **When** they look for knowledge retrieval guidance, **Then** they find examples of cross-repo velocity trends and retrospective outcome patterns.

---

### User Story 3 - Visitor understands Dewey as a team member (Priority: P2)

A visitor browsing the team section wants to understand Dewey's role in the AI agent swarm. They find a dedicated team page that explains Dewey's architecture, capabilities, and how each hero persona uses it.

**Why this priority**: The team page provides the conceptual overview. Less urgent than actionable guides but important for understanding the ecosystem.

**Independent Test**: The Dewey team page covers all sections present in other hero pages and accurately describes Dewey's 40 MCP tools, 3 content source types, and semantic search capabilities as of v0.2.0.

**Acceptance Scenarios**:

1. **Given** a visitor navigates to the team section, **When** they click on Dewey, **Then** they see a page describing Dewey's role as the semantic knowledge layer with its key capabilities.
2. **Given** a visitor reads the Dewey team page, **When** they look for integration details, **Then** they find a section describing how each hero persona uses Dewey for their specific needs.

---

### User Story 4 - Common workflows reference includes Dewey context (Priority: P2)

A user consulting the common-workflows reference understands how Dewey provides knowledge context at each stage of the hero lifecycle.

**Why this priority**: The workflows page is the comprehensive reference for users who already understand the basics.

**Independent Test**: The common-workflows page mentions Dewey in the knowledge context stage and explains the graceful degradation model.

**Acceptance Scenarios**:

1. **Given** a user reads the new feature workflow, **When** they reach the knowledge context section, **Then** they understand that Dewey provides automatic semantic context at every swarm stage.
2. **Given** a user reads the environment setup workflow, **When** they follow the setup steps, **Then** Dewey installation is included as a setup step.

---

### User Story 5 - Dewey listed as a project (Priority: P3)

A visitor browsing the projects section finds Dewey listed alongside Gaze with a project page describing its purpose, installation, and status.

**Why this priority**: The projects section provides discoverability but is not the primary documentation path.

**Independent Test**: A `projects/dewey.md` page exists with project description, installation command, key features, and link to the getting-started guide.

**Acceptance Scenarios**:

1. **Given** a visitor browses the projects section, **When** they see the project listing, **Then** Dewey appears alongside Gaze as a current project.
2. **Given** a visitor clicks on the Dewey project, **When** the page loads, **Then** they see a summary of Dewey's purpose, key features, and a link to the full getting-started guide.

---

### Edge Cases

- What happens when documentation references Dewey features that require Ollama but Ollama is not installed? Documentation must explain the graceful degradation: semantic search tools return clear error messages, all keyword-based tools continue to work.
- What happens when the website references `brew install --cask` but the user is on Linux? Documentation must note that Homebrew cask is macOS-only and provide the alternative `go install` path for Linux.
- What happens when existing page content already covers some Dewey topics? All existing content must be preserved and refined; Dewey sections are updates to existing pages, not replacements.

## Requirements *(mandatory)*

### Functional Requirements

**Getting-Started Knowledge Guide** (knowledge.md):

- **FR-001**: The knowledge guide MUST explain Dewey's purpose in plain language — what it does and why developers need it.
- **FR-002**: The knowledge guide MUST provide complete installation steps: Homebrew cask install, Ollama install, embedding model pull, and `dewey init`.
- **FR-003**: The knowledge guide MUST explain how to configure content sources (local disk, GitHub API, web crawl) with YAML configuration examples.
- **FR-004**: The knowledge guide MUST explain OpenCode MCP integration with a JSON configuration example.
- **FR-005**: The knowledge guide MUST describe the 3-tier graceful degradation model (full Dewey, reduced context, functional fallback).
- **FR-006**: The knowledge guide MUST include a Linux installation alternative (`go install`) since Homebrew cask is macOS-only.

**Team Page** (team/dewey.md):

- **FR-007**: The Dewey team page MUST describe Dewey's architecture at a high level (MCP server, SQLite persistence, Ollama embeddings, pluggable sources).
- **FR-008**: The Dewey team page MUST list the primary query capabilities (40 MCP tools across 9 categories).
- **FR-009**: The Dewey team page MUST describe how each of the 5 hero personas uses Dewey with role-specific examples.
- **FR-010**: The Dewey team page MUST describe the 3 content source types and the embedding model (IBM Granite).

**Role Guides** (developer.md, tester.md, product-owner.md, product-manager.md):

- **FR-011**: Each role guide MUST include a "Knowledge Retrieval with Dewey" subsection with 2-3 concrete query examples specific to that role.
- **FR-012**: Each role guide's Dewey subsection MUST link to the getting-started knowledge guide for full setup instructions.

**Common Workflows** (common-workflows.md):

- **FR-013**: The common-workflows page MUST include Dewey's role in the knowledge context stage of the hero lifecycle.
- **FR-014**: The environment setup workflow MUST include Dewey installation as a setup step.

**Projects Page** (projects/dewey.md):

- **FR-015**: A Dewey project page MUST exist in the projects section with project description, installation command, key features, and link to the getting-started guide.
- **FR-016**: The projects index MUST list Dewey alongside Gaze as a current project.

**Cross-Cutting**:

- **FR-017**: All new and updated pages MUST follow the existing website writing style: direct, declarative, no emojis in prose, second person for guides, no marketing superlatives.
- **FR-018**: All new pages MUST include proper Hugo frontmatter (title, description, lead, date, weight, toc).
- **FR-019**: All new pages MUST end with a "Next Steps" section containing 2-4 cross-links to related pages.
- **FR-020**: All content MUST be accurate to Dewey v0.2.0 capabilities — no references to unimplemented features.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: A developer unfamiliar with Dewey can follow the getting-started guide from zero to a working `dewey serve` session in under 10 minutes.
- **SC-002**: All 4 role guides (developer, tester, product-owner, product-manager) include a Dewey subsection with at least 2 concrete query examples each.
- **SC-003**: The Dewey team page describes all 5 hero persona interactions with role-specific examples.
- **SC-004**: The common-workflows page includes Dewey in the knowledge context stage and environment setup.
- **SC-005**: A `projects/dewey.md` page exists and is linked from the projects index.
- **SC-006**: All new and updated pages pass the website's existing markdown lint rules.
- **SC-007**: Zero references to unimplemented Dewey features appear in any documentation page.

## Assumptions

- The website repo is at `unbound-force/website` and uses Hugo with the Doks theme.
- Dewey v0.2.0 is the current released version and all documented features match its capabilities.
- The `knowledge.md` and `team/dewey.md` pages already have content from earlier work — updates should preserve and refine existing content rather than replacing it.
- The website follows a consistent writing style and page structure — new content must match this exactly.
- The `brew install --cask unbound-force/tap/dewey` command is the primary macOS installation method; `go install` is the alternative for Linux.
- Implementation of this spec happens in the `unbound-force/website` repo, not the dewey repo. This spec drives the work; the code changes happen in the website repo.
