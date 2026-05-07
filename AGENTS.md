# AGENTS.md

## Project Overview

Dewey is a knowledge graph MCP server that gives AI agents full access to Markdown knowledge bases. It supports **Logseq** and **Obsidian** with full read-write support — 50 MCP tools across navigate, search, analyze, write, decision, journal, flashcard, whiteboard, semantic search, compile, lint, promote, indexing, learning, and curate categories. Hard fork of [graphthulhu](https://github.com/skridlevsky/graphthulhu), extended with persistent SQLite storage, vector-based semantic search via Ollama, pluggable content sources (disk, GitHub, web crawl, code), knowledge compilation with temporal intelligence, curated knowledge stores, pluggable LLM providers (Ollama, Vertex AI), and trust tiers.

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

### Gatekeeping Value Protection

Agents MUST NOT modify values that serve as quality or
governance gates to make an implementation pass. The
following categories are protected:

1. **Coverage thresholds and CRAP scores** — minimum
   coverage percentages, CRAP score limits, coverage
   ratchets
2. **Severity definitions and auto-fix policies** —
   CRITICAL/HIGH/MEDIUM/LOW boundaries, auto-fix
   eligibility rules
3. **Convention pack rule classifications** —
   MUST/SHOULD/MAY designations on convention pack rules
   (downgrading MUST to SHOULD is prohibited)
4. **CI flags and linter configuration** — `-race`,
   `-count=1`, `govulncheck`, `golangci-lint` rules,
   pinned action SHAs
5. **Agent temperature and tool-access settings** —
   frontmatter `temperature`, `tools.write`, `tools.edit`,
   `tools.bash` restrictions
6. **Constitution MUST rules** — any MUST rule in
   `.specify/memory/constitution.md` or hero constitutions
7. **Review iteration limits and worker concurrency** —
   max review iterations, max concurrent Swarm workers,
   retry limits
8. **Workflow gate markers** — `<!-- spec-review: passed
   -->`, task completion checkboxes used as gates, phase
   checkpoint requirements

**What to do instead**: When an implementation cannot
meet a gate, the agent MUST stop, report which gate is
blocking and why, and let the human decide whether to
adjust the gate or rework the implementation. Modifying
a gate without explicit human authorization is a
constitution violation (CRITICAL severity).

### Workflow Phase Boundaries

Agents MUST NOT cross workflow phase boundaries:

- **Specify/Clarify/Plan/Tasks/Analyze/Checklist** phases:
  spec artifacts ONLY (`specs/NNN-*/` directory). No
  source code, test, agent, command, or config changes.
- **Implement** phase: source code changes allowed,
  guided by spec artifacts.
- **Review** phase: findings and minor fixes only. No new
  features.

A phase boundary violation is treated as a process error.
The agent MUST stop and report the violation rather than
proceeding with out-of-phase changes.

## Technical Guardrails

- **CI Parity Gate**: Before marking any implementation task complete or declaring a PR ready, agents MUST replicate the CI checks locally. Read `.github/workflows/` to identify the exact commands CI runs, then execute those same commands. Any failure is a blocking error — a task is not complete until all CI-equivalent checks pass locally. Do not rely on a memorized list of commands; always derive them from the workflow files, which are the source of truth.
- **No CGO**: All dependencies MUST be pure Go. The constitution prohibits CGO unless no pure-Go alternative exists.
- **Local-Only Processing**: No data leaves the developer's machine by default. Embedding generation uses Ollama locally. Cloud providers (Vertex AI) are opt-in via `config.yaml`.
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

### Website Documentation Sync

When a change adds, modifies, or removes user-facing behavior (new commands, flags, output formats, MCP tools, configuration fields, or installation steps), a GitHub issue **must** be created in the `unbound-force/website` repository documenting what changed and what website pages need updating. This ensures the public documentation stays in sync with the codebase.

The issue should include:
- Which dewey feature/command changed
- Which website pages are affected (reference `content/docs/` paths when known)
- What specifically needs updating (new section, changed example, removed content)

Use `gh issue create --repo unbound-force/website` to create the issue. The issue title should follow the format: `docs: sync dewey <feature> documentation`.

Exempt from this requirement:
- Internal refactors with no user-facing changes
- Test-only changes
- Spec artifact updates (these live in the dewey repo, not the website)

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
| `--vault PATH` | | Path to vault (on serve, index, reindex, status, search, doctor, manifest) |

### CLI Commands

| Command | Description |
|---------|-------------|
| `dewey serve` | Start the MCP server (default when no subcommand) |
| `dewey init` | Initialize `.uf/dewey/` directory with config and sources |
| `dewey index` | Build or update the knowledge graph index from configured sources |
| `dewey reindex` | Delete and rebuild the index from scratch |
| `dewey status` | Show index status (page/block/link counts, source info) |
| `dewey search` | Full-text search across the knowledge graph |
| `dewey source` | Manage content sources (add, list, remove) |
| `dewey doctor` | Run diagnostic checks on the index and environment |
| `dewey manifest` | Generate `.uf/dewey/manifest.md` — a structured summary of CLI commands, MCP tools, and exported packages discovered via AST parsing of Go source files |
| `dewey journal` | Append a block to a Logseq journal page |
| `dewey add` | Append a block to a named Logseq page |
| `dewey compile` | Synthesize stored learnings into compiled knowledge articles |
| `dewey lint` | Scan the knowledge base for quality issues (stale decisions, embedding gaps, contradictions, knowledge store quality) |
| `dewey promote` | Promote a page from `draft` tier to `validated` |
| `dewey curate` | Run the curation pipeline to extract structured knowledge from indexed sources into knowledge stores |

### Content Source Types

| Type | Description | Required Config |
|------|-------------|-----------------|
| `disk` | Local markdown files | `path` |
| `github` | GitHub issues, PRs, discussions | `org`, `repos` |
| `web` | Web page crawling | `urls` |
| `code` | Source code indexing via language-aware AST chunking | `path`, `languages` |

The `code` source type uses the `chunker/` package to parse source files and extract high-signal blocks (exported functions, types, constants, Cobra commands, MCP tool registrations). Each source file produces one Document with markdown-formatted declarations. Test files (e.g., `*_test.go`) are automatically excluded.

## Architecture

MCP server + CLI tool with flat package layout:

```text
main.go              # Entry point, Cobra root command, serve logic
cli.go               # CLI subcommands (journal, add, search, init, index, reindex, status, source, doctor, manifest, compile, lint, promote, curate)
server.go            # MCP server setup, 50 tool registrations
backend/             # Backend interface + capability interfaces
client/              # Logseq HTTP API client with retry/backoff
vault/               # Obsidian vault backend (file parsing, indexing, watcher, persistence)
vault/parse_export.go # Exported parsing and persistence functions (ParseDocument, PersistBlocks, PersistLinks, GenerateEmbeddings)
tools/               # MCP tool implementations (navigate, search, analyze, write, decision, journal, flashcard, whiteboard, semantic, compile, lint, promote, curate)
types/               # Shared types (PageEntity, BlockEntity, tool inputs, semantic search types)
llm/                 # LLM synthesis interface (Synthesizer, OllamaSynthesizer, VertexSynthesizer, NoopSynthesizer for tests)
parser/              # Content parser (wikilinks, tags, properties)
graph/               # In-memory graph construction + algorithms
store/               # SQLite persistence layer (pages, blocks, links, embeddings, sources)
embed/               # Embedding generation (Ollama + Vertex AI clients, provider factory, config, chunker)
source/              # Pluggable content sources (disk, GitHub, web crawl, code, manager)
chunker/             # Language-aware source code parsing (Chunker interface, Go implementation, registry)
curate/              # Knowledge store configuration parsing + curation pipeline (config, extraction, quality analysis)
```

### Key Patterns

- **Backend interface**: All MCP tools program against `backend.Backend`, not concrete implementations. Adding a new backend (e.g., Dendron) requires no changes to existing tools.
- **Optional persistence**: The `store.Store` is passed via `vault.WithStore()` option. When nil, Dewey operates in-memory (graphthulhu-compatible mode).
- **Graceful degradation**: Semantic search tools return clear error messages when Ollama is unavailable. All keyword-based tools continue to work.
- **Cobra CLI**: Root command doubles as `serve` for backward compatibility.
- **charmbracelet/log**: Structured logging throughout. No `fmt.Fprintf` to stderr.
- **Trust tiers**: Pages have a `tier` field (`authored`, `curated`, `validated`, `draft`) and optional `category` field for knowledge provenance tracking (013-knowledge-compile, 015-curated-knowledge-stores).
- **Background indexing**: The MCP server starts before vault indexing completes. Tools serve from the persistent store (previous session's data) during background indexing. An `atomic.Bool` `indexReady` flag tracks completion (012-background-index).
- **Ollama auto-start**: `ensureOllama()` detects Ollama state (External/Managed/Unavailable) and auto-starts a subprocess if installed but not running. The subprocess is detached via `Setpgid` so it outlives Dewey (007-ollama-autostart). Only triggered when the embedding provider is `ollama`.
- **Pluggable providers**: Both `Embedder` and `Synthesizer` interfaces support multiple backends (ollama, vertex). Configured via `config.yaml` `embedding` and `synthesis` sections. Factory functions `embed.NewEmbedderFromConfig()` and `llm.NewSynthesizerFromConfig()` centralize construction. Backward compatible — existing configs and env vars continue to work.
- **Vertex AI rate limiting**: Both `VertexEmbedder` and `VertexSynthesizer` retry on HTTP 429 with exponential backoff (base 1s, max 60s, up to 5 attempts). Respects the `Retry-After` header when present. Context cancellation is honored between retries. Retries are logged at warn level via `charmbracelet/log`.

### Provider Configuration

Embedding and synthesis backends are configurable in `.uf/dewey/config.yaml`:

```yaml
embedding:
  provider: ollama  # or: vertex
  model: granite-embedding:30m
  endpoint: http://localhost:11434

synthesis:
  provider: vertex
  model: claude-sonnet-4-6
  project: my-gcp-project
  region: us-east5
```

Vertex providers use `golang.org/x/oauth2/google` application-default credentials. If no provider is specified, defaults to Ollama.

**Global config**: `~/.config/dewey/config.yaml` (or `$XDG_CONFIG_HOME/dewey/config.yaml`) provides defaults for all vaults. Per-vault config overrides global. This avoids repeating provider config in every project.

### Store Learning API

The `store_learning` MCP tool stores knowledge with temporal awareness:

- **`tag`** (required): Topic namespace (e.g., `authentication`, `vault-walker`). Used for clustering and identity.
- **`category`** (optional): One of `decision`, `pattern`, `gotcha`, `context`, `reference`. Guides compilation strategy.
- **`information`** (required): Natural language paragraph describing the learning.
- **Returns**: `{tag}-{YYYYMMDDTHHMMSS}-{author}` identity (e.g., `authentication-20260502T143022-alice`), using UTC timestamps and resolved author identity.
- **Automatic fields**: `created_at` (ISO 8601), `tier` (defaults to `draft`), `author` (resolved via three-tier fallback: `DEWEY_AUTHOR` env var → `git config user.name` → `"anonymous"`).

Set `DEWEY_AUTHOR` to override the default author identity (useful in CI or shared environments where git config may not be available).

Backward compatibility: the old `tags` (plural, comma-separated) field is still accepted — the first tag is used.

### Trust Tiers

| Tier | Source | Description |
|------|--------|-------------|
| `authored` | disk, GitHub, web, code sources | Human-written content. Highest trust. Default for all indexed sources. |
| `curated` | `dewey curate`, knowledge stores | Machine-extracted knowledge from indexed sources. LLM-curated with quality flags and confidence scores. |
| `validated` | `dewey promote` | Agent content promoted by human review. Middle trust. |
| `draft` | `store_learning`, `dewey compile` | Agent-generated content. Unreviewed. Default for learnings and compiled articles. |

Filter by tier: `semantic_search_filtered(query: "auth", tier: "authored")` returns only human-written content.
Filter by tier: `semantic_search_filtered(query: "auth", tier: "curated")` returns only knowledge store content.

### Knowledge Stores

Knowledge stores are named collections of curated knowledge extracted from indexed sources. Configured in `.uf/dewey/knowledge-stores.yaml`:

```yaml
stores:
  - name: team-knowledge
    sources:
      - disk-meetings
      - github-org
    path: .uf/dewey/knowledge/team-knowledge  # default
    curation_interval: "10m"                   # default
    curate_on_index: false                     # default
```

- **`name`**: Unique identifier for the store
- **`sources`**: List of source IDs from `sources.yaml` to curate from
- **`path`**: Output directory for curated markdown files (defaults to `.uf/dewey/knowledge/{name}`)
- **`curation_interval`**: How often background curation runs (default: `10m`)
- **`curate_on_index`**: Whether to curate automatically after indexing

The curation pipeline uses an LLM (via Ollama) to extract structured knowledge items (decisions, patterns, gotchas, context, references) from indexed content. Each item includes confidence scoring (`high`/`medium`/`low`/`flagged`) and quality flags (`missing_rationale`, `implied_assumption`, `incongruent`, `unsupported_claim`).

Curated files are automatically indexed as a `knowledge-{store-name}` source with `tier: "curated"`, making them immediately searchable via `semantic_search` and other MCP tools.

### File-Backed Learnings

Learnings stored via `store_learning` are dual-written to both SQLite and markdown files at `.uf/dewey/learnings/{tag}-{YYYYMMDDTHHMMSS}-{author}.md`. This ensures learnings survive `graph.db` deletion — on startup, orphaned markdown files are re-ingested automatically.

### Background Curation

During `dewey serve`, a background goroutine periodically checks for new indexed content and curates incrementally. The curation interval is configurable per store. Background curation shares a mutex with indexing to prevent concurrent operations.

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

## Knowledge Retrieval

Agents SHOULD prefer Dewey MCP tools over grep/glob/read
for cross-repo context, design decisions, and
architectural patterns. Dewey provides semantic search
across all indexed Markdown files, specs, and web
documentation — returning ranked results with provenance
metadata that grep cannot match.

### Tool Selection Matrix

| Query Intent | Dewey Tool | When to Use |
|-------------|-----------|-------------|
| Conceptual understanding | `semantic_search` | "How does X work?" |
| Keyword lookup | `search` | Known terms, FR numbers |
| Read specific page | `get_page` | Known document path |
| Relationship discovery | `find_connections` | "How are X and Y related?" |
| Similar documents | `similar` | "Find specs like this one" |
| Tag-based discovery | `find_by_tag` | "All pages tagged #decision" |
| Property queries | `query_properties` | "All specs with status: draft" |
| Filtered semantic | `semantic_search_filtered` | Semantic search within source type |
| Graph navigation | `traverse` | Dependency chain walking |

### When to Fall Back to grep/glob/read

Use direct file operations instead of Dewey when:
- **Dewey is unavailable** — MCP tools return errors or
  are not configured
- **Exact string matching is needed** — searching for a
  specific error message, variable name, or code pattern
- **Specific file path is known** — reading a file you
  already know the path to (use Read directly)
- **Binary/non-Markdown content** — Dewey indexes
  Markdown; use grep for Go source, JSON, YAML, etc.

### Graceful Degradation (3-Tier Pattern)

**Tier 3 (Full Dewey)** — semantic + structured search:
- `semantic_search` — natural language queries
- `search` — keyword queries
- `get_page`, `find_connections`, `traverse` — structured navigation
- `find_by_tag`, `query_properties` — metadata queries

**Tier 2 (Graph-only, no embedding model)** — structured
search only:
- `search` — keyword queries (no embeddings needed)
- `get_page`, `traverse`, `find_connections` — graph navigation
- `find_by_tag`, `query_properties` — metadata queries
- Semantic search unavailable — use exact keyword matches

**Tier 1 (No Dewey)** — direct file access:
- Use Read tool for direct file access
- Use Grep for keyword search across the codebase
- Use Glob for file pattern matching

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
|------|---------|--------------|--------|
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
  010-code-source-index/       # Code source indexing, Go chunker, manifest generation (In Progress)
  013-knowledge-compile/       # Knowledge compilation, temporal intelligence, linting, trust tiers (Complete)
  015-curated-knowledge-stores/ # File-backed learnings, curation pipeline, knowledge stores, curated tier (In Progress)
  016-pluggable-providers/       # Pluggable embedding/synthesis providers (Ollama, Vertex AI), store_compiled, global config (Complete)
```

## Active Technologies
- SQLite via `modernc.org/sqlite` -- single database `.uf/dewey/graph.db` containing the knowledge graph index (pages, blocks, links) and vector embeddings (001-core-implementation)
- Go 1.25 (per `go.mod`) + Gaze v1.4.6 (`go install github.com/unbound-force/gaze/cmd/gaze@latest`) (002-quality-ratchets)
- N/A (quality improvement, no storage changes) (002-quality-ratchets)
- Go 1.25 (per `go.mod`) + `modernc.org/sqlite` (pure-Go SQLite), `github.com/modelcontextprotocol/go-sdk` (MCP), `github.com/spf13/cobra` (CLI), `github.com/charmbracelet/log` (logging), `github.com/k3a/html2text` (web crawl) (004-unified-content-serve)
- SQLite via `modernc.org/sqlite` — single database `.uf/dewey/graph.db` containing pages, blocks, links, embeddings, sources, metadata tables (004-unified-content-serve)
- Go 1.25 (per `go.mod`) + `github.com/spf13/cobra` (CLI), `github.com/charmbracelet/log` (logging), `github.com/mattn/go-runewidth` (terminal width — already used by summary box) (005-doctor-emoji-markers)
- N/A (no storage changes) (005-doctor-emoji-markers)
- Go 1.25 (per `go.mod`) + `github.com/fsnotify/fsnotify` (file watcher), `github.com/spf13/cobra` (CLI), `github.com/charmbracelet/log` (logging), `gopkg.in/yaml.v3` (config parsing) (006-unified-ignore)
- N/A (no storage changes — this feature modifies filesystem walking, not the SQLite store) (006-unified-ignore)
- Go 1.25 (per `go.mod`) + `os/exec` (subprocess), `net/http` (health check), `github.com/charmbracelet/log` (logging), `github.com/spf13/cobra` (CLI) (007-ollama-autostart)
- Go 1.25 (per `go.mod`) + `go/parser`, `go/ast`, `go/token`, `go/format` (all stdlib — AST parsing for code chunking), `github.com/spf13/cobra` (CLI), `github.com/charmbracelet/log` (logging) (010-code-source-index)
- N/A (no storage changes — code source documents flow through existing SQLite pipeline) (010-code-source-index)
- Go 1.25 (per `go.mod`) + `github.com/modelcontextprotocol/go-sdk` (MCP SDK), `github.com/unbound-force/dewey/source` (source manager), `github.com/unbound-force/dewey/store` (SQLite persistence), `github.com/unbound-force/dewey/embed` (Ollama embeddings), `github.com/unbound-force/dewey/vault` (document parsing/persistence), `sync` (mutex for mutual exclusion) (011-live-reindex)
- N/A (no storage changes — MCP tools wrap existing indexing pipeline, no new tables or schema changes) (011-live-reindex)
- Go 1.25 (per `go.mod`) + `sync/atomic` (readiness flag), `sync` (shared mutex), `github.com/modelcontextprotocol/go-sdk` (MCP SDK), `github.com/charmbracelet/log` (logging) (012-background-index)
- N/A (no storage changes — restructures startup sequence, no new tables or schema changes) (012-background-index)
- Go 1.25 (per `go.mod`) + `modernc.org/sqlite` (pure-Go SQLite, schema migration v1→v2), `github.com/modelcontextprotocol/go-sdk` (MCP SDK — 3 new tools: compile, lint, promote), `github.com/spf13/cobra` (CLI — 3 new commands), `github.com/charmbracelet/log` (logging), `github.com/unbound-force/dewey/embed` (Ollama embeddings for clustering), `net/http` (Ollama /api/generate for LLM synthesis) (013-knowledge-compile)
- SQLite schema v1→v2: `pages` table gains `tier TEXT DEFAULT 'authored'` and `category TEXT` columns + `idx_pages_tier` index. New `llm/` package for LLM synthesis interface. (013-knowledge-compile)

- Go 1.25 (per `go.mod`) + `modernc.org/sqlite` v1.47.0 (pure-Go SQLite), `github.com/k3a/html2text` v1.4.0 (HTML-to-text for web crawl), `github.com/modelcontextprotocol/go-sdk` v1.2.0 (existing MCP SDK), `github.com/spf13/cobra` (CLI framework), `github.com/charmbracelet/log` (structured logging) (001-core-implementation)
- Go 1.25 (per `go.mod`) + `gopkg.in/yaml.v3` (knowledge store config parsing), `github.com/unbound-force/dewey/llm` (LLM synthesis for curation), `github.com/unbound-force/dewey/curate` (new package — config parsing + curation pipeline), `github.com/spf13/cobra` (CLI — `dewey curate` command), `github.com/modelcontextprotocol/go-sdk` (MCP SDK — `curate` tool) (015-curated-knowledge-stores)
- N/A (no schema changes — `curated` is a new value in the existing `tier TEXT` column. File-backed learnings use filesystem, not new tables) (015-curated-knowledge-stores)

## Recent Changes
- 016-pluggable-providers: Added Vertex AI embedding and synthesis providers, `store_compiled` MCP tool, global config fallback (`~/.config/dewey/config.yaml`), factory functions `embed.NewEmbedderFromConfig()` and `llm.NewSynthesizerFromConfig()`, `golang.org/x/oauth2/google` dependency
- learning-identity-collision-fix (OpenSpec): Changed learning identity format from `{tag}-{sequence}` to `{tag}-{YYYYMMDDTHHMMSS}-{author}` with three-tier author resolution (`DEWEY_AUTHOR` env var → `git config user.name` → `"anonymous"`), sub-second collision avoidance via `O_CREATE|O_EXCL` with suffix fallback, removed `NextLearningSequence` from store, backward-compatible re-ingestion of old-format files
- 015-curated-knowledge-stores: Added `curate/` package (config parsing + curation pipeline), `dewey curate` CLI command, `curate` MCP tool, file-backed learnings (`.uf/dewey/learnings/`), `curated` trust tier, knowledge stores (`.uf/dewey/knowledge-stores.yaml`), background curation goroutine, lint knowledge store quality metrics
- 012-background-index: Added Go 1.25 (per `go.mod`) + `sync/atomic` (readiness flag), `sync` (shared mutex)
- 002-quality-ratchets: Added Go 1.25 (per `go.mod`)
- 001-core-implementation: Added Go 1.25 (per `go.mod`) + `modernc.org/sqlite` v1.47.0 (pure-Go SQLite), `github.com/k3a/html2text` v1.4.0 (HTML-to-text for web crawl), `github.com/modelcontextprotocol/go-sdk` v1.2.0 (existing MCP SDK), `github.com/spf13/cobra` (CLI framework), `github.com/charmbracelet/log` (structured logging)

- 001-core-implementation: Added Go 1.25 (per `go.mod`) + `modernc.org/sqlite` v1.47.0 (pure-Go SQLite), `github.com/k3a/html2text` v1.4.0 (HTML-to-text for web crawl), `github.com/modelcontextprotocol/go-sdk` v1.2.0 (existing MCP SDK)

<!-- MANUAL ADDITIONS START -->
<!-- MANUAL ADDITIONS END -->
<!-- scaffolded by unbound vdev -->
<!-- scaffolded by unbound vdev -->
