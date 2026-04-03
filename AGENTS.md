# AGENTS.md

## Project Overview

Dewey is a knowledge graph MCP server that gives AI agents full access to Markdown knowledge bases. It supports **Logseq** and **Obsidian** with full read-write support — 40 MCP tools across navigate, search, analyze, write, decision, journal, flashcard, whiteboard, and semantic search categories. Hard fork of [graphthulhu](https://github.com/skridlevsky/graphthulhu), extended with persistent SQLite storage, vector-based semantic search via Ollama, and pluggable content sources (disk, GitHub, web crawl).

- **Language**: Go 1.25+
- **Module**: `github.com/unbound-force/dewey`
- **License**: MIT (original graphthulhu) + Unbound Force copyright

## Core Mission

- **Strategic Architecture**: Engineers shift from manual coding to directing an "infinite supply of junior developers" (AI agents).
- **Outcome Orientation**: Focus on conveying business value and user intent rather than low-level technical sub-tasks.
- **Intent-to-Context**: Treat specs and rules as the medium through which human intent is manifested into code.

## Behavioral Constraints

- **Zero-Waste Mandate**: No orphaned code, unused dependencies, or "Feature Zombie" bloat.
- **Neighborhood Rule**: Changes must be audited for negative impacts on adjacent modules or the wider ecosystem. The 37 inherited graphthulhu MCP tools must continue to work identically after any change.
- **Intent Drift Detection**: Evaluation must detect when the implementation drifts away from the original human-written "Statement of Intent."
- **Automated Governance**: Primary feedback is provided via automated constraints (CI, Gaze quality gates, constitution checks), reserving human energy for high-level security and logic.

## Technical Guardrails

- **CI Parity Gate**: Before marking any implementation task complete or declaring a PR ready, agents MUST replicate the CI checks locally. Read `.github/workflows/` to identify the exact commands CI runs, then execute those same commands. Any failure is a blocking error — a task is not complete until all CI-equivalent checks pass locally. Do not rely on a memorized list of commands; always derive them from the workflow files, which are the source of truth.
- **No CGO**: All dependencies MUST be pure Go. The constitution prohibits CGO unless no pure-Go alternative exists.
- **Local-Only Processing**: No data MUST leave the developer's machine. Embedding generation uses Ollama locally.
- **Backward Compatibility**: All 37 inherited graphthulhu MCP tools MUST produce identical results after any change.

## Council Governance Protocol

- **The Architect**: Must verify that "Intent Driving Implementation" is maintained.
- **The Adversary**: Acts as the primary "Automated Governance" gate for security.
- **The Guard**: Detects "Intent Drift" to ensure the business value remains intact.
- **The Tester**: Must verify that test quality, coverage strategy, and testability are maintained.
- **The Operator**: Audits deployment and operational readiness.

**Rule**: A Pull Request is only "Ready for Human" once the `/review-council` command returns an **APPROVE** status from all reviewers.

### Review Council as PR Prerequisite

Before submitting a pull request, agents **must** run `/review-council` and resolve all REQUEST CHANGES findings until all reviewers return APPROVE. There must be **minimal to no code changes** between the council's APPROVE verdict and the PR submission — the council reviews the final code, not a draft that changes afterward.

Workflow:
1. Complete all implementation tasks
2. Run CI checks locally (build, lint, vet, test)
3. Run `/review-council` — fix any findings, re-run until APPROVE
4. Commit, push, and submit PR immediately after council APPROVE
5. Do NOT make further code changes between APPROVE and PR submission

Exempt from council review:
- Constitution amendments (governance documents, not code)
- Documentation-only changes (README, AGENTS.md, spec artifacts)
- Emergency hotfixes (must be retroactively reviewed)

## Spec-First Development (Mandatory)

All changes that modify production code, test code, agent prompts, embedded assets, or CI configuration **must** be preceded by a spec workflow. The constitution (`.specify/memory/constitution.md`) is the highest-authority document in this project — all work must align with it.

Two spec workflows are available:

| Workflow | Location | Best For |
|----------|----------|----------|
| **Speckit** | `specs/NNN-name/` | Numbered feature specs with the full pipeline (specify → clarify → plan → tasks → implement) |
| **OpenSpec** | `openspec/changes/name/` | Targeted changes with lightweight artifacts (proposal → design → specs → tasks) via `/opsx-propose` and `/opsx-apply` |

**What requires a spec** (no exceptions without explicit user override):
- New features or capabilities
- Refactoring that changes function signatures, extracts helpers, or moves code between packages
- Test additions or assertion strengthening across multiple functions
- CI workflow modifications
- Data model changes (new struct fields, schema updates)

**What is exempt** (may be done directly):
- Constitution amendments (governed by the constitution's own Governance section)
- Typo corrections, comment-only changes, single-line formatting fixes
- Emergency hotfixes for critical production bugs (must be retroactively documented)

When an agent is unsure whether a change is trivial, it **must** ask the user rather than proceeding without a spec. The cost of an unnecessary spec is minutes; the cost of an unplanned change is rework, drift, and broken CI.

### Pipeline

The workflow is a strict, sequential pipeline. Each stage has a corresponding `/speckit.*` command:

```text
constitution → specify → clarify → plan → tasks → analyze → checklist → implement
```

| Command | Purpose |
|---------|---------|
| `/speckit.constitution` | Create or update the project constitution |
| `/speckit.specify` | Create a feature specification from a description |
| `/speckit.clarify` | Reduce ambiguity in the spec before planning |
| `/speckit.plan` | Generate the technical implementation plan |
| `/speckit.tasks` | Generate actionable, dependency-ordered task list |
| `/speckit.analyze` | Non-destructive cross-artifact consistency analysis |
| `/speckit.checklist` | Generate requirement quality validation checklists |
| `/speckit.implement` | Execute the implementation plan task by task |

### Ordering Constraints

1. Constitution must exist before specs.
2. Spec must exist before plan.
3. Plan must exist before tasks.
4. Tasks must exist before implementation and analysis.
5. Clarify should run before plan (skipping increases rework risk).
6. Analyze should run after tasks but before implementation.
7. All checklists must pass before implementation (or user must explicitly override).

### Strategic vs Tactical

| Criterion | Speckit (Strategic) | OpenSpec (Tactical) |
|-----------|:------------------:|:-------------------:|
| User stories | >= 3 | < 3 |
| Cross-repo impact | Yes | No |
| New MCP tools | Always | Never |
| Bug fix | Never | Always |
| Single-package maintenance | Never | Usually |

When in doubt, start with OpenSpec. If scope grows beyond 3 stories, escalate to Speckit.

### Branch Conventions

Both tiers enforce branch-based workflows:

- **Speckit** branches: `NNN-<short-name>`
  (e.g., `013-binary-rename`). Created automatically by
  `/speckit.specify`. Validated by `check-prerequisites.sh`
  at every pipeline step (hard gate).
- **OpenSpec** branches: `opsx/<change-name>`
  (e.g., `opsx/doctor-ux-improvement`). Created by
  `/opsx-propose`. Validated by `/opsx-apply` before
  implementation (hard gate).

The `opsx/` prefix namespace ensures OpenSpec branches
are visually distinct from Speckit branches in
`git branch` output and do not collide with the
`NNN-*` numbering pattern.

### Task Completion Bookkeeping

When a task from `tasks.md` is completed during implementation, its checkbox **must** be updated from `- [ ]` to `- [x]` immediately. Do not defer this — mark tasks complete as they are finished, not in a batch after all work is done.

### Documentation Validation Gate

Before marking any task complete, you **must** validate whether the change requires documentation updates. Check and update as needed:

- `README.md` — new/changed commands, flags, output formats, or architecture
- `AGENTS.md` — new conventions, packages, patterns, or workflow changes
- GoDoc comments — new or modified exported functions, types, and packages
- Spec artifacts under `specs/` — if the change affects planned behavior

A task is not complete until its documentation impact has been assessed and any necessary updates have been made.

### Spec Commit Gate

All spec artifacts (`spec.md`, `plan.md`, `tasks.md`, and any other files under `specs/`) **must** be committed and pushed before implementation begins. Run `/speckit.implement` only after the spec commit is on the remote.

### Constitution Check

A mandatory gate at the planning phase. The constitution's four core principles — Composability First, Autonomous Collaboration, Observable Quality, and Testability — must each receive a PASS before proceeding. Constitution violations are automatically CRITICAL severity and non-negotiable.

## Core Principles

These principles (from the project constitution) guide all development:

1. **Composability First**: Dewey MUST be independently installable and usable without any other Unbound Force tool. Graceful degradation when Dewey tools are unavailable.
2. **Autonomous Collaboration**: All communication via MCP tool calls. No runtime coupling, shared memory, or direct function calls.
3. **Observable Quality**: Every result includes provenance metadata. Index state is auditable via `health` tool and `dewey status`.
4. **Testability**: Every package testable in isolation. Coverage ratchets enforced by CI. Missing coverage strategy is CRITICAL.

## Build & Test Commands

```bash
# Build
go build ./...

# Run all tests
go test -race -count=1 ./...

# Run tests with coverage (for Gaze)
go test -race -count=1 -coverprofile=coverage.out ./...

# Static analysis
go vet ./...

# Gaze quality report (local)
gaze report ./... --coverprofile=coverage.out --max-crapload=48 --max-gaze-crapload=18 --min-contract-coverage=70
```

Always run tests with `-race -count=1`. CI enforces this.

### Global CLI Flags

| Flag | Short | Description |
|------|-------|-------------|
| `--verbose` | `-v` | Enable debug logging (UUID seeds, block insertions, lock detection) |
| `--log-file PATH` | | Write logs to file in addition to stderr |
| `--no-embeddings` | | Skip embedding generation (on serve, index, reindex) |
| `--vault PATH` | | Path to vault (on serve, index, reindex, status, search, doctor) |

## Architecture

MCP server + CLI tool with flat package layout:

```text
main.go              # Entry point, Cobra root command, serve logic
cli.go               # CLI subcommands (journal, add, search, init, index, reindex, status, source, doctor)
server.go            # MCP server setup, 40 tool registrations
backend/             # Backend interface + capability interfaces
client/              # Logseq HTTP API client with retry/backoff
vault/               # Obsidian vault backend (file parsing, indexing, watcher, persistence)
vault/parse_export.go # Exported parsing and persistence functions (ParseDocument, PersistBlocks, PersistLinks, GenerateEmbeddings)
tools/               # MCP tool implementations (navigate, search, analyze, write, decision, journal, flashcard, whiteboard, semantic)
types/               # Shared types (PageEntity, BlockEntity, tool inputs, semantic search types)
parser/              # Content parser (wikilinks, tags, properties)
graph/               # In-memory graph construction + algorithms
store/               # SQLite persistence layer (pages, blocks, links, embeddings, sources)
embed/               # Embedding generation (Ollama client, chunker)
source/              # Pluggable content sources (disk, GitHub, web crawl, manager)
```

### Key Patterns

- **Backend interface**: All MCP tools program against `backend.Backend`, not concrete implementations. Adding a new backend (e.g., Dendron) requires no changes to existing tools.
- **Optional persistence**: The `store.Store` is passed via `vault.WithStore()` option. When nil, Dewey operates in-memory (graphthulhu-compatible mode).
- **Graceful degradation**: Semantic search tools return clear error messages when Ollama is unavailable. All keyword-based tools continue to work.
- **Cobra CLI**: Root command doubles as `serve` for backward compatibility.
- **charmbracelet/log**: Structured logging throughout. No `fmt.Fprintf` to stderr.

## Coding Conventions

- **Formatting**: `gofmt` and `goimports` (enforced by golangci-lint via MegaLinter).
- **Naming**: Standard Go conventions. PascalCase for exported, camelCase for unexported.
- **Comments**: GoDoc-style comments on all exported functions and types.
- **Error handling**: Return `error` values. Wrap with `fmt.Errorf("context: %w", err)`.
- **Import grouping**: Standard library, then third-party, then internal packages (separated by blank lines).
- **No global state**: The logger is the only package-level variable. Prefer dependency injection.
- **SQL safety**: All store operations MUST use parameterized queries. Never interpolate user content into SQL strings.
- **Logging**: Use `github.com/charmbracelet/log`. No `fmt.Fprintf(os.Stderr, ...)`.
- **CLI Framework**: Use `github.com/spf13/cobra`. No `flag.FlagSet`.

## Testing Conventions

- **Framework**: Standard library `testing` package only. No testify, gomega, or other external assertion libraries.
- **Assertions**: Use `t.Errorf` / `t.Fatalf` directly. No assertion helpers from third-party packages.
- **Test naming**: `TestXxx_Description` (e.g., `TestStore_InsertPage`, `TestSemanticSearch_EmptyIndex`).
- **Test files**: `*_test.go` alongside source in the same directory.
- **Test isolation**: Use in-memory SQLite (`:memory:`) for store tests. Use `httptest` for HTTP client tests. Use `t.TempDir()` for filesystem tests.
- **Mock backend**: `tools/mock_backend_test.go` provides a shared `mockBackend` implementing `backend.Backend` for all tool tests.
- **Race detection**: Always run with `-race` flag.
- **Coverage ratchets**: CI enforces quality thresholds via Gaze (`--max-crapload`, `--max-gaze-crapload`, `--min-contract-coverage`).

## Git & Workflow

- **Commit format**: Conventional Commits — `type: description` (e.g., `feat:`, `fix:`, `docs:`, `chore:`, `refactor:`).
- **Branching**: Feature branches required. No direct commits to `main` except trivial doc fixes.
- **Code review**: Required before merge.
- **Semantic versioning**: For releases.

## CI/CD

Three GitHub Actions workflows:

1. **CI** (`.github/workflows/ci.yml`): Build + vet + test with `-race -count=1` + Gaze quality report with threshold enforcement on push/PR.
2. **MegaLinter** (`.github/workflows/mega-linter.yml`): Runs golangci-lint, markdownlint, yamllint, and gitleaks on push/PR to `main`. Auto-commits lint fixes to PR branches.
3. **Release** (`.github/workflows/release.yml`): Triggered on `v*` tag push. Runs GoReleaser to build cross-platform binaries (darwin/linux x amd64/arm64), create GitHub Releases, and update the Homebrew formula in `unbound-force/homebrew-tap`.

## Sibling Repositories

| Repo | Purpose | Constitution | Status |
|------|---------|-------------|--------|
| `unbound-force/unbound-force` | Meta repo (specs, governance, CLI) | v1.1.0 (org constitution) | Active |
| `unbound-force/gaze` | Go static analysis (tester hero) | v1.3.0 (Accuracy, Minimal Assumptions, Actionable Output, Testability) | Active |
| `unbound-force/website` | Public website (Hugo + Doks) | v1.0.0 (Content Accuracy, Minimal Footprint, Visitor Clarity) | Active |
| `unbound-force/homebrew-tap` | Homebrew formula distribution | N/A | Active |

## Spec Organization

Specs are numbered with 3-digit zero-padded prefixes:

```text
specs/
  001-core-implementation/     # Persistence, vector search, content sources, CLI (Complete)
  002-quality-ratchets/        # Gaze CI, CRAPload reduction, contract coverage (In Progress)
```

## Active Technologies
- SQLite via `modernc.org/sqlite` -- single database `.dewey/graph.db` containing the knowledge graph index (pages, blocks, links) and vector embeddings (001-core-implementation)
- Go 1.25 (per `go.mod`) + Gaze v1.4.6 (`go install github.com/unbound-force/gaze/cmd/gaze@latest`) (002-quality-ratchets)
- N/A (quality improvement, no storage changes) (002-quality-ratchets)
- Go 1.25 (per `go.mod`) + `modernc.org/sqlite` (pure-Go SQLite), `github.com/modelcontextprotocol/go-sdk` (MCP), `github.com/spf13/cobra` (CLI), `github.com/charmbracelet/log` (logging), `github.com/k3a/html2text` (web crawl) (004-unified-content-serve)
- SQLite via `modernc.org/sqlite` — single database `.dewey/graph.db` containing pages, blocks, links, embeddings, sources, metadata tables (004-unified-content-serve)
- Go 1.25 (per `go.mod`) + `github.com/spf13/cobra` (CLI), `github.com/charmbracelet/log` (logging), `github.com/mattn/go-runewidth` (terminal width — already used by summary box) (005-doctor-emoji-markers)
- N/A (no storage changes) (005-doctor-emoji-markers)
- Go 1.25 (per `go.mod`) + `github.com/fsnotify/fsnotify` (file watcher), `github.com/spf13/cobra` (CLI), `github.com/charmbracelet/log` (logging), `gopkg.in/yaml.v3` (config parsing) (006-unified-ignore)
- N/A (no storage changes — this feature modifies filesystem walking, not the SQLite store) (006-unified-ignore)
- Go 1.25 (per `go.mod`) + `os/exec` (subprocess), `net/http` (health check), `github.com/charmbracelet/log` (logging), `github.com/spf13/cobra` (CLI) (007-ollama-autostart)

- Go 1.25 (per `go.mod`) + `modernc.org/sqlite` v1.47.0 (pure-Go SQLite), `github.com/k3a/html2text` v1.4.0 (HTML-to-text for web crawl), `github.com/modelcontextprotocol/go-sdk` v1.2.0 (existing MCP SDK), `github.com/spf13/cobra` (CLI framework), `github.com/charmbracelet/log` (structured logging) (001-core-implementation)

## Recent Changes
- 002-quality-ratchets: Added Go 1.25 (per `go.mod`)
- 001-core-implementation: Added Go 1.25 (per `go.mod`) + `modernc.org/sqlite` v1.47.0 (pure-Go SQLite), `github.com/k3a/html2text` v1.4.0 (HTML-to-text for web crawl), `github.com/modelcontextprotocol/go-sdk` v1.2.0 (existing MCP SDK), `github.com/spf13/cobra` (CLI framework), `github.com/charmbracelet/log` (structured logging)

- 001-core-implementation: Added Go 1.25 (per `go.mod`) + `modernc.org/sqlite` v1.47.0 (pure-Go SQLite), `github.com/k3a/html2text` v1.4.0 (HTML-to-text for web crawl), `github.com/modelcontextprotocol/go-sdk` v1.2.0 (existing MCP SDK)

<!-- MANUAL ADDITIONS START -->
<!-- MANUAL ADDITIONS END -->
<!-- scaffolded by unbound vdev -->
<!-- scaffolded by unbound vdev -->
