# Tasks: Website Documentation for Dewey

**Input**: Design documents from `/specs/003-website-docs/`
**Prerequisites**: plan.md (required), spec.md (required for user stories), research.md

**Tests**: No automated tests. Verification is via markdownlint, link validation, Hugo build, and content accuracy review.

**Organization**: Tasks are grouped by user story. US1 and US2 are both P1 but US1 (knowledge guide) should be done first since US2 role guides link to it. US3 and US4 are P2 and can run in parallel. US5 is P3.

**Implementation repo**: `unbound-force/website` (not the dewey repo where this spec lives).

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies on incomplete tasks)
- **[Story]**: Which user story this task belongs to (e.g., US1, US2, US3)
- Include exact file paths in descriptions

---

## Phase 1: Setup

**Purpose**: Prepare the website repo for Dewey documentation work

- [ ] T001 Read existing content in the website repo to understand current state: content/docs/getting-started/knowledge.md, content/docs/team/dewey.md, content/docs/getting-started/developer.md, content/docs/getting-started/tester.md, content/docs/getting-started/product-owner.md, content/docs/getting-started/product-manager.md, content/docs/getting-started/common-workflows.md, content/docs/projects/gaze.md (as template for dewey project page), content/docs/projects/_index.md
- [ ] T002 Read Dewey v0.2.0 CLI help output to verify all documented commands and flags are accurate: `dewey --help`, `dewey serve --help`, `dewey init --help`, `dewey index --help`, `dewey status --help`, `dewey source --help`

**Checkpoint**: Current content and Dewey v0.2.0 capabilities understood. Ready to write content.

---

## Phase 2: User Story 1 -- Getting-Started Knowledge Guide (Priority: P1)

**Goal**: A developer unfamiliar with Dewey can follow the knowledge guide from zero to a working `dewey serve` session in under 10 minutes.

**Independent Test**: Follow the guide step-by-step on a fresh machine and verify each command works as documented.

### Implementation for User Story 1

- [ ] T003 [US1] Update content/docs/getting-started/knowledge.md: Write "What Dewey Does" section explaining Dewey as a semantic knowledge layer that combines structured graph traversal with vector-based semantic search (FR-001)
- [ ] T004 [US1] Update content/docs/getting-started/knowledge.md: Write "Installation" section with macOS Homebrew cask (`brew install --cask unbound-force/tap/dewey`), Linux `go install` alternative, Ollama install, and `ollama pull granite-embedding:30m` (FR-002, FR-006)
- [ ] T005 [US1] Update content/docs/getting-started/knowledge.md: Write "Initialize Your Repository" section with `dewey init` command and explanation of `.dewey/` directory structure (FR-002)
- [ ] T006 [US1] Update content/docs/getting-started/knowledge.md: Write "Configure Content Sources" section with YAML examples for disk, GitHub API, and web crawl sources in `.dewey/sources.yaml` (FR-003)
- [ ] T007 [US1] Update content/docs/getting-started/knowledge.md: Write "OpenCode Integration" section with JSON MCP config example showing `dewey serve --vault .` (FR-004)
- [ ] T008 [US1] Update content/docs/getting-started/knowledge.md: Write "Graceful Degradation" section with 3-tier table: full Dewey (semantic + structured), reduced (Dewey without Ollama, structured only), fallback (no Dewey, direct file reads) (FR-005)
- [ ] T009 [US1] Update content/docs/getting-started/knowledge.md: Write "Next Steps" section with links to role guides (developer, tester, product-owner, product-manager) and team/dewey page (FR-019)
- [ ] T010 [US1] Verify content/docs/getting-started/knowledge.md has correct Hugo frontmatter (title, description, lead, date, weight: 80, toc: true) and matches website writing style (FR-017, FR-018)

**Checkpoint**: Knowledge guide is complete. A developer can follow it to install, configure, and run Dewey.

---

## Phase 3: User Story 2 -- Role-Specific Dewey Subsections (Priority: P1)

**Goal**: Each role guide includes a "Knowledge Retrieval with Dewey" subsection with 2-3 concrete query examples.

**Independent Test**: Read any single role guide and find a Dewey subsection with role-specific examples.

### Implementation for User Story 2

- [ ] T011 [P] [US2] Update content/docs/getting-started/developer.md: Add "Knowledge Retrieval with Dewey" subsection with examples: "How does Cobra handle subcommand validation?", "Find implementations of retry logic across repos", link to knowledge guide (FR-011, FR-012)
- [ ] T012 [P] [US2] Update content/docs/getting-started/tester.md: Add "Knowledge Retrieval with Dewey" subsection with examples: "What test patterns exist for HTTP client mocking?", "Find quality baselines from sibling projects", link to knowledge guide (FR-011, FR-012)
- [ ] T013 [P] [US2] Update content/docs/getting-started/product-owner.md: Add "Knowledge Retrieval with Dewey" subsection with examples: "Find open issues about authentication across the org", "What acceptance criteria were used for similar features?", link to knowledge guide (FR-011, FR-012)
- [ ] T014 [P] [US2] Update content/docs/getting-started/product-manager.md: Add "Knowledge Retrieval with Dewey" subsection with examples: "What quality trends emerged across projects this quarter?", "Find retrospective outcomes for velocity improvements", link to knowledge guide (FR-011, FR-012)

**Checkpoint**: All 4 role guides have Dewey subsections with role-specific examples.

---

## Phase 4: User Story 3 -- Dewey Team Page (Priority: P2)

**Goal**: The Dewey team page describes Dewey's architecture, 40 MCP tools, 3 content sources, and how each hero uses it.

**Independent Test**: The team page covers all sections present in other hero pages and accurately describes v0.2.0 capabilities.

### Implementation for User Story 3

- [ ] T015 [US3] Update content/docs/team/dewey.md: Write/update role summary and "What It Does" sections describing Dewey as the semantic knowledge layer with 40 MCP tools across 9 categories (navigate, search, analyze, write, decision, journal, flashcard, whiteboard, semantic) (FR-007, FR-008)
- [ ] T016 [US3] Update content/docs/team/dewey.md: Write/update "Content Sources" section describing disk (filesystem walk, SHA-256 hashing), GitHub API (issues, PRs, READMEs, token precedence), and web crawl (BFS, robots.txt, HTML-to-text, cache) sources (FR-010)
- [ ] T017 [US3] Update content/docs/team/dewey.md: Write/update "Embedding Model" section describing IBM Granite (30M, 63 MB, Apache 2.0, local via Ollama, no data leaves machine) (FR-010)
- [ ] T018 [US3] Update content/docs/team/dewey.md: Write/update "How Heroes Use Dewey" section with per-hero subsections for Muti-Mind (cross-repo issues, past specs), Cobalt-Crush (API docs, implementation patterns), Gaze (test patterns, quality baselines), The Divisor (convention context, review findings), Mx F (velocity trends, retrospectives) (FR-009)
- [ ] T019 [US3] Verify content/docs/team/dewey.md has correct Hugo frontmatter and "Next Steps" section linking to knowledge guide and projects page (FR-018, FR-019)

**Checkpoint**: Dewey team page is complete with all sections matching hero page format.

---

## Phase 5: User Story 4 -- Common Workflows Update (Priority: P2)

**Goal**: The common-workflows page includes Dewey's role in knowledge context and environment setup.

**Independent Test**: The workflows page mentions Dewey in the new feature workflow and includes Dewey in setup steps.

### Implementation for User Story 4

- [ ] T020 [US4] Update content/docs/getting-started/common-workflows.md: Add or update "Knowledge Context" subsection in the New Feature workflow explaining how Dewey provides automatic semantic context at every swarm stage (FR-013)
- [ ] T021 [US4] Update content/docs/getting-started/common-workflows.md: Add Dewey installation step to the Environment Setup workflow (install Dewey, install Ollama, pull embedding model) (FR-014)

**Checkpoint**: Common workflows reference includes Dewey context and setup.

---

## Phase 6: User Story 5 -- Dewey Project Page (Priority: P3)

**Goal**: Dewey is listed as a project alongside Gaze with a dedicated project page.

**Independent Test**: A `projects/dewey.md` page exists and is listed in the projects index.

### Implementation for User Story 5

- [ ] T022 [US5] Create content/docs/projects/dewey.md modeled after content/docs/projects/gaze.md: include Hugo frontmatter (title: "Dewey", weight: 30), project description, `brew install --cask unbound-force/tap/dewey` installation command, key features (40 MCP tools, semantic search, 3 content sources, persistent SQLite index), link to getting-started guide (FR-015, FR-018)
- [ ] T023 [US5] Update content/docs/projects/_index.md: Add Dewey to the "Current Projects" list alongside Gaze with a brief description and link to projects/dewey page (FR-016)

**Checkpoint**: Dewey project page exists and is discoverable from the projects section.

---

## Phase 7: Polish & Cross-Cutting Concerns

- [ ] T024 Run markdownlint on all new and modified files to verify lint compliance (SC-006)
- [ ] T025 Run `hugo --minify` in the website repo to verify no Hugo build errors
- [ ] T026 Verify all internal cross-links resolve: check every `/docs/...` link in new/modified pages
- [ ] T027 Verify content accuracy: cross-reference all `dewey` commands, flags, and YAML config keys against `dewey --help` output (FR-020, SC-007)
- [ ] T028 Verify writing style consistency: confirm no emojis in prose, no superlatives, second person for guides, third person for team/project pages (FR-017)

**Checkpoint**: All pages pass lint, build, link validation, accuracy, and style checks. Ready for PR.

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies — read existing content
- **US1 Knowledge Guide (Phase 2)**: Depends on Setup — must be done before US2 (role guides link to it)
- **US2 Role Guides (Phase 3)**: Depends on US1 — all 4 role guide tasks can run in parallel
- **US3 Team Page (Phase 4)**: Depends on Setup only — can run in parallel with US1/US2
- **US4 Common Workflows (Phase 5)**: Depends on Setup only — can run in parallel with US3
- **US5 Project Page (Phase 6)**: Depends on Setup only — can run in parallel with US3/US4
- **Polish (Phase 7)**: Depends on all user stories being complete

### User Story Dependencies

- **US1 (P1)**: Independent. Must complete before US2.
- **US2 (P1)**: Depends on US1 (links to knowledge guide). 4 tasks run in parallel.
- **US3 (P2)**: Independent. Can run in parallel with US1/US2.
- **US4 (P2)**: Independent. Can run in parallel with US3.
- **US5 (P3)**: Independent. Can run in parallel with US3/US4.

---

## Parallel Example: User Story 2

```bash
# All 4 role guide updates in parallel (different files, same pattern):
Task: "T011 [P] [US2] Update developer.md with Dewey subsection"
Task: "T012 [P] [US2] Update tester.md with Dewey subsection"
Task: "T013 [P] [US2] Update product-owner.md with Dewey subsection"
Task: "T014 [P] [US2] Update product-manager.md with Dewey subsection"
```

## Parallel Example: US3 + US4 + US5

```bash
# These three user stories can all run in parallel (different files):
Task: "T015-T019 [US3] Update team/dewey.md"
Task: "T020-T021 [US4] Update common-workflows.md"
Task: "T022-T023 [US5] Create projects/dewey.md + update _index.md"
```

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Complete Phase 1: Setup (read existing content)
2. Complete Phase 2: US1 Knowledge Guide
3. **STOP and VALIDATE**: A developer can follow the guide to install and run Dewey
4. Deploy if ready — the knowledge guide alone delivers value

### Incremental Delivery

1. US1 (knowledge guide) → core onboarding value
2. US2 (role guides) → role-specific value, 4 parallel tasks
3. US3 + US4 + US5 (team + workflows + project) → ecosystem context, all parallel
4. Polish → lint, build, links, accuracy

---

## Notes

- Implementation happens in `unbound-force/website`, not the dewey repo
- [P] tasks = different files, no dependencies on incomplete tasks
- [Story] label maps task to specific user story for traceability
- All content must be accurate to Dewey v0.2.0 — verify with `dewey --help`
- Existing pages already have partial Dewey content — update, don't replace
- Follow website writing style exactly: direct, declarative, no emojis, second person for guides
