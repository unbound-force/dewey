# AGENTS.md

## Project Overview

Dewey is a knowledge graph MCP server that gives AI agents full access to Markdown knowledge bases. It supports **Logseq** and **Obsidian** with full read-write support — 50 MCP tools across navigate, search, analyze, write, decision, journal, flashcard, whiteboard, semantic search, compile, lint, promote, indexing, learning, and curate categories. Hard fork of [graphthulhu](https://github.com/skridlevsky/graphthulhu), extended with persistent SQLite storage, vector-based semantic search via Ollama, pluggable content sources (disk, GitHub, web crawl, code), knowledge compilation with temporal intelligence, curated knowledge stores, pluggable LLM providers (Ollama, Vertex AI, any OpenAI-compatible gateway), and trust tiers.

- **Language**: Go 1.25+
- **Module**: `github.com/unbound-force/dewey/v3`
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
- **Local-Only Processing**: No data leaves the developer's machine by default. Embedding generation uses Ollama locally. Cloud providers (Vertex AI) and OpenAI-compatible gateways (OpenRouter, ENMaas, LiteLLM, vLLM, self-hosted) are opt-in via `config.yaml`; gateway credentials are environment-only.
- **Backward Compatibility**: All 37 inherited graphthulhu MCP tools MUST produce identical results after any change.

## Council Governance Protocol

- **The Architect**: Must verify that "Intent Driving Implementation" is maintained.
- **The Adversary**: Acts as the primary "Automated Governance" gate for security.
- **The Guard**: Detects "Intent Drift" to ensure the business value remains intact.
- **The Tester**: Must verify that test quality, coverage strategy, and testability are maintained.
- **The Operator**: Audits deployment and operational readiness.

**Rule**: A Pull Request is only "Ready for Human" once the `/uf.review-council` command returns an **APPROVE** status from all reviewers.

### Review Council as PR Prerequisite

Before submitting a pull request, agents **must** run `/uf.review-council` and resolve all REQUEST CHANGES findings until all reviewers return APPROVE. There must be **minimal to no code changes** between the council's APPROVE verdict and the PR submission — the council reviews the final code, not a draft that changes afterward.

Workflow:
1. Complete all implementation tasks
2. Run CI checks locally (build, lint, vet, test)
3. Run `/uf.review-council` — fix any findings, re-run until APPROVE
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

Use `gh issue create --repo unbound-force/website --label docs` to create the issue. The issue title should follow the format: `docs: sync dewey <feature> documentation`. The `docs` label keeps sync issues consistent with their siblings (e.g., website#41, #113, #335).

Exempt from this requirement:
- Internal refactors with no user-facing changes
- Test-only changes
- Spec artifact updates (these live in the dewey repo, not the website)

### Commit Scope (Learnings)

Learnings written to `.uf/dewey/learnings/` (by `store_learning`, review-dispatch
proposal storage, or `dewey compile` runs) are non-rebuildable knowledge
artifacts, so they stay tracked on `main`: gitignoring them would risk silent
data loss when `graph.db` is deleted without a backup remote (dewey-publish
design). They MUST NOT be committed on a feature or PR branch:

- Leave new learnings untracked on feature branches. They reach `main`
  through a separate `chore/dewey-learnings-*` PR, which gh-hud opens once a
  day per checkout (open one by hand without it). `uf_finale_stage` and
  `/uf.address-feedback` never stage them.
- Compiled articles (`.uf/dewey/compiled/*.md`) produced during a change
  still ride the feature PR.
- Keep `.uf/dewey/learnings/` OUT of `.gitignore`.
- Before committing learnings, screen them for credentials/secrets (the
  gitleaks MegaLinter job scans `.uf/` as a backstop, but do not rely on it
  alone).
- Prefer `dewey artifacts plan` to review what would be published, and use
  `dewey artifacts sync` when publishing stranded artifacts deliberately —
  it adds credential screening and audit trailers behind a human gate.
- Never force publication through the staging sweep: if a learning is not
  ready to be public, leave it uncommitted.

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
gaze crap --format=json --coverprofile=coverage.out --max-crapload=48 --max-gaze-crapload=45 ./...
```

Always run tests with `-race -count=1`. CI enforces this.

### Global CLI Flags

| Flag | Short | Description |
|------|-------|-------------|
| `--verbose` | `-v` | Enable debug logging (UUID seeds, block insertions, lock detection) |
| `--log-file PATH` | | Write logs to file in addition to stderr |
| `--no-embeddings` | | Skip embedding generation (on serve, index, reindex) |
| `--vault PATH` | | Path to vault (on serve, index, reindex, status, search, doctor, manifest, artifacts plan, artifacts sync) |

### CLI Commands

| Command | Description |
|---------|-------------|
| `dewey serve` | Start the MCP server (default when no subcommand) |
| `dewey init` | Initialize `.uf/dewey/` directory with config and sources; when `.opencode/` exists, also scaffolds `.opencode/agents/curator.md` (never overwriting an existing file) |
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
| `dewey lint` | Scan the knowledge base for quality issues (stale decisions, embedding gaps, contradictions, knowledge store quality, `anchor_drift`/`anchor_missing` on `as_of`-pinned learnings; `--fix` marks drifted learnings `stale-unverified`) |
| `dewey promote` | Promote one or more pages from `draft` to `validated` (batch is all-or-nothing; names resolve exact, then case-insensitive, then bare tags against `learning/`, `compiled/`, and `knowledge/`, then source doc IDs; ambiguous matches are refused) |
| `dewey curate` | Run the curation pipeline to extract structured knowledge from indexed sources into knowledge stores |
| `dewey artifacts` | Plan and publish stranded knowledge artifacts to git behind a human gate |
| `dewey gate` | Deterministic startup gates (`dewey gate auto-answer` refuses when the curator agent is configured for automatic question answering) |

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
cli.go               # CLI subcommands (journal, add, search, init, index, reindex, status, source, doctor, manifest, compile, lint, promote, curate, artifacts)
gate.go              # CLI gate subcommands (dewey gate auto-answer — deterministic startup gates)
agents.go            # Shipped agent definitions (scaffolded into .opencode/agents/ by dewey init)
server.go            # MCP server setup, 50 tool registrations
backend/             # Backend interface + capability interfaces
client/              # Logseq HTTP API client with retry/backoff
vault/               # Obsidian vault backend (file parsing, indexing, watcher, persistence)
vault/parse_export.go # Exported parsing and persistence functions (ParseDocument, PersistBlocks, PersistLinks, GenerateEmbeddings)
tools/               # MCP tool implementations (navigate, search, analyze, write, decision, journal, flashcard, whiteboard, semantic, compile, lint, promote, curate)
types/               # Shared types (PageEntity, BlockEntity, tool inputs, semantic search types)
llm/                 # LLM synthesis interface (Synthesizer, OllamaSynthesizer, VertexSynthesizer, OpenAICompatibleSynthesizer, NoopSynthesizer for tests)
openaicompat/        # Shared OpenAI-compatible transport (endpoint, auth, retry, errors, client, availability)
parser/              # Content parser (wikilinks, tags, properties)
graph/               # In-memory graph construction + algorithms
store/               # SQLite persistence layer (pages, blocks, links, embeddings, sources)
embed/               # Embedding generation (Ollama, Vertex AI, and OpenAI-compatible clients; provider factory, config, chunker, openai_compatible.go)
source/              # Pluggable content sources (disk, GitHub, web crawl, code, manager)
chunker/             # Language-aware source code parsing (Chunker interface, Go implementation, registry)
curate/              # Knowledge store configuration parsing + curation pipeline (config, extraction, quality analysis)
sanitize/            # Content sanitization pipeline (injection pattern scanning, structure validation, drift detection, size anomaly detection)
artifacts/           # Knowledge-artifact publishing: plan/sync sweep, screening, gitignore posture
```

### Key Patterns

- **Backend interface**: All MCP tools program against `backend.Backend`, not concrete implementations. Adding a new backend (e.g., Dendron) requires no changes to existing tools.
- **Optional persistence**: The `store.Store` is passed via `vault.WithStore()` option. When nil, Dewey operates in-memory (graphthulhu-compatible mode).
- **Live store read-through**: When a store is configured, vault read operations (`GetPage`, `GetPageBlocksTree`, `GetBlock`, `GetAllPages`, `FullTextSearch`, tag search) query the persistent store on an in-memory miss — no mid-session caching, vault files take precedence, and a nil store behaves as an always-miss (no-store parity, Constitution II). On the store lookup, a bare name (no `/`) resolves exact → `compiled/<name>` → `learning/<name>`, first match wins — reads only; the write guard and `promote` keep exact/strict resolution (read-tools-bare-names). Store-written pages (`store_learning`, `store_compiled`) are visible to read tools in the same session without a restart, and `Reload()` re-runs the store page load so store-backed pages survive reloads (mcp-internal-store-access).
- **Graceful degradation**: Semantic search tools return clear error messages when Ollama is unavailable. All keyword-based tools continue to work.
- **Cobra CLI**: Root command doubles as `serve` for backward compatibility.
- **charmbracelet/log**: Structured logging throughout. No `fmt.Fprintf` to stderr.
- **Trust tiers**: Pages have a `tier` field (`authored`, `curated`, `validated`, `draft`, `untrusted`) and optional `category` field for knowledge provenance tracking (013-knowledge-compile, 015-curated-knowledge-stores). The `untrusted` tier is for content from sources explicitly marked as lower trust by the user.
- **Content sanitization**: The `sanitize` package scans indexed content for adversarial injection patterns, structural anomalies (invisible Unicode, data URIs, suspicious HTML), content hash drift, and size anomalies. Configured per-source via `sanitize_mode` (`warn`/`strict`/`off`) and `trust_tier` in `sources.yaml`. Findings are merged into page properties and surfaced by `dewey doctor` and `dewey lint`.
- **Background indexing**: The MCP server starts before vault indexing completes. Tools serve from the persistent store (previous session's data) during background indexing. An `atomic.Bool` `indexReady` flag tracks completion (012-background-index).
- **Ollama auto-start**: `ensureOllama()` detects Ollama state (External/Managed/Unavailable) and auto-starts a subprocess if installed but not running. The subprocess is detached via `Setpgid` so it outlives Dewey (007-ollama-autostart). Only triggered when the embedding provider is `ollama`.
- **Pluggable providers**: Both `Embedder` and `Synthesizer` interfaces support multiple backends (ollama, vertex, openai-compatible). Configured via `config.yaml` `embedding` and `synthesis` sections. Factory functions `embed.NewEmbedderFromConfig()` and `llm.NewSynthesizerFromConfig()` centralize construction. Embedding speaks the OpenAI v1 wire protocol through the shared `openaicompat/` transport (normalization, environment-only credentials, 429 retry, redacted errors, cached `/models` probe); the synthesis provider owns its own transport in `llm/` and resolves its key from `DEWEY_SYNTHESIS_API_KEY` then `synthesis.api_key`. Backward compatible — existing configs and env vars continue to work.
- **Server-side synthesis**: `executeServe` builds one synthesizer and passes it through `WithSynthesizer` to `compile` and `curate` (and background curation). Compile shares the pipeline index mutex with indexing and curation. `/dewey-distill` drives lint → inventory → scope gate → compile/curate → review gate → batch promote entirely through MCP tools, because the running server owns the store lock. The command binds to the `curator` agent (`agent: curator` frontmatter) and runs `dewey gate auto-answer` at startup, refusing on any non-pass verdict; the gate invocation is the single documented shell exception to the MCP-only rule because it never touches the store lock. Both mandatory gates carry the `HUMAN_GATE:` marker so auto-answer plugins honoring the external contract leave them for a human.
- **Vertex AI rate limiting**: Both `VertexEmbedder` and `VertexSynthesizer` retry on HTTP 429 with exponential backoff (base 1s, max 60s, up to 5 attempts). Respects the `Retry-After` header when present. Context cancellation is honored between retries. Retries are logged at warn level via `charmbracelet/log`.

### Provider Configuration

Embedding and synthesis backends are configurable in `.uf/dewey/config.yaml`:

```yaml
embedding:
  provider: ollama  # or: vertex, openai-compatible
  model: granite-embedding:30m
  endpoint: http://localhost:11434
  max_chunk_chars: 12288  # optional: override chunk size limit

synthesis:
  provider: vertex
  model: claude-sonnet-4-6
  project: my-gcp-project
  region: us-east5
```

OpenAI-compatible gateways use `provider: openai-compatible` with `model` (sent verbatim) and `endpoint` (the API base URL). There is no Ollama fallback for gateway endpoints:

```yaml
embedding:
  provider: openai-compatible
  model: openrouter/amazon/titan-embed-text-v2
  endpoint: https://openrouter.ai/api/v1

synthesis:
  provider: openai-compatible
  model: enmaas/rits/zai-org/glm-5-3
  endpoint: https://gateway.example/v1
```

Embedding gateway credentials are environment-only and never read from `config.yaml`: `DEWEY_EMBEDDING_API_KEY`, then `DEWEY_API_KEY`, then `OPENROUTER_API_KEY` only when the endpoint host is exactly `openrouter.ai`. An empty value counts as unset and no key is sent when none resolves. An `api_key` field under `embedding:` in `config.yaml` is ignored and warned about at startup and in `dewey doctor`, which also warns when a resolved key would transit `http://` to a non-loopback host. Endpoint precedence keeps the documented asymmetry: embedding is env-first, synthesis is config-first; `OLLAMA_HOST` and the localhost default never apply to gateways.

An OpenAI-compatible provider (for example enmaas) performs synthesis server-side in the running MCP server:

```yaml
synthesis:
  provider: openai-compatible
  endpoint: https://api.enmaas.devshift.net/v1
  model: rits/zai-org/glm-5-3
  timeout: 300s       # optional
  max_tokens: 16000   # optional
```

The synthesis API key resolves from `DEWEY_SYNTHESIS_API_KEY` first, then `synthesis.api_key`. The per-vault `.uf/dewey/config.yaml` is git-tracked (`dewey init` keeps it shareable), so keep secrets in the environment variable or the global `~/.config/dewey/config.yaml`. It is never logged or committed; provider error bodies are scrubbed of the key. `dewey serve` builds the synthesizer once at startup and passes it to the `compile` and `curate` MCP tools (and background curation). HTTP 429 retries use the Vertex backoff policy, and `finish_reason: "length"` is reported as a truncation error. When no provider is configured or construction fails, both tools keep the prompt-mode fallback.

Vertex providers use `golang.org/x/oauth2/google` application-default credentials. If no provider is specified, defaults to Ollama. The `region` field accepts any GCP region (e.g., `us-east5`, `us-central1`) or `global` to route requests to the nearest available region.

**Embedding endpoint resolution** (highest to lowest precedence):
1. `DEWEY_EMBEDDING_ENDPOINT` env var (app-specific override)
2. `config.yaml` `embedding.endpoint` (per-vault, then global)
3. `OLLAMA_HOST` env var (ecosystem-standard fallback)
4. `http://localhost:11434` (default)

When `OLLAMA_HOST` is set without a URL scheme (e.g., `0.0.0.0:11434`), `http://` is prepended automatically.

**Synthesis endpoint resolution** (highest to lowest precedence):
1. `config.yaml` `synthesis.endpoint` (per-vault, then global)
2. `DEWEY_SYNTHESIS_ENDPOINT` env var (app-specific override)
3. `OLLAMA_HOST` env var (ecosystem-standard fallback)
4. `http://localhost:11434` (default)

Note: Unlike embedding (where the env var overrides config.yaml), the synthesis path gives `config.yaml` highest precedence for provider/model/endpoint. Env vars are consulted only when `config.yaml` does not specify a `synthesis` section. This asymmetry exists because `ReadSynthesisConfig()` checks config file values first and returns early, while the env var fallback chain (`ResolveSynthesisEndpoint()`) is only consulted for the legacy `compile_model` path and the env-only fallback. The OpenAI-compatible **API key** is the exception: `DEWEY_SYNTHESIS_API_KEY` overrides `synthesis.api_key`.

When `OLLAMA_HOST` or `DEWEY_SYNTHESIS_ENDPOINT` is set without a URL scheme (e.g., `0.0.0.0:11434`), `http://` is prepended automatically.

**Chunk size limit resolution** (highest to lowest precedence):
1. `DEWEY_CHUNK_MAX_CHARS` env var
2. `config.yaml` `embedding.max_chunk_chars` (per-vault, then global)
3. Default: `12288` (~8192 tokens at 1.5 chars/token)

The chunk size limit controls the maximum character length of text chunks sent to the embedding model. Different models have different context windows — adjust this value to match your model. Invalid values (non-positive or non-numeric) are logged as a warning and fall back to the default.

**Graceful degradation**: When Ollama is reachable but the configured embedding model has not been pulled, Dewey logs a warning and continues in keyword-only mode instead of exiting. Semantic search MCP tools return clear error messages indicating embeddings are unavailable.

**Global config**: `~/.config/dewey/config.yaml` (or `$XDG_CONFIG_HOME/dewey/config.yaml`) provides defaults for all vaults. Per-vault config overrides global. This avoids repeating provider config in every project.

### Store Learning API

The `store_learning` MCP tool stores knowledge with temporal awareness:

- **`tag`** (required): Topic namespace (e.g., `authentication`, `vault-walker`). Used for clustering and identity.
- **`category`** (optional): One of `decision`, `pattern`, `gotcha`, `context`, `reference`. Guides compilation strategy.
- **`information`** (required): Natural language paragraph describing the learning.
- **`evidence`** (optional): Up to 50 locator strings backing the learning (`<repo>:<path>`, `<repo>#<issue-or-PR>`, `learning/<identity>`, `session:<id>`); each is trimmed, deduplicated, and capped at 512 characters (counted in runes, not bytes).
- **`validity`** (optional): One of `current`, `stale-unverified`, `superseded`, `historical`. Absent means unknown — there is no default.
- **`supersedes`** (optional): Identity (or `learning/<identity>`) of an existing learning this one replaces. The target gains `superseded_by` and `validity: superseded` in the store and in its markdown backup; superseded learnings are never deleted and each has at most one successor — enforced at write time by a compare-and-swap store claim plus a guarded markdown backup replace, not merely at validation. If the back-link fails after the successor is stored, the result carries a non-empty `supersedes_warning`; a concurrent same-target supersession whose claim loses also yields `supersedes_warning`, naming the winning successor, while the losing learning is kept with its `supersedes` claim and the recorded successor is not overwritten.
- **Returns**: `{tag}-{YYYYMMDDTHHMMSS}-{author}` identity (e.g., `authentication-20260502T143022-alice`), using UTC timestamps and resolved author identity.
- **Automatic fields**: `created_at` (ISO 8601), `tier` (defaults to `draft`), `author` (resolved via three-tier fallback: `DEWEY_AUTHOR` env var → `git config user.name` → `"anonymous"`), `as_of` (`<repo>@<HEAD sha>` of the vault's git repository; omitted entirely when the vault is not a git repo or git fails — no fallback).

Set `DEWEY_AUTHOR` to override the default author identity (useful in CI or shared environments where git config may not be available).

Backward compatibility: the old `tags` (plural, comma-separated) field is still accepted — the first tag is used. Temporal-validity fields round-trip through markdown frontmatter and startup re-ingestion; legacy files without them re-ingest unchanged.

### Trust Tiers

| Tier | Source | Description |
|------|--------|-------------|
| `authored` | disk, GitHub, web, code sources | Human-written content. Highest trust. Default for all indexed sources. |
| `curated` | `dewey curate`, knowledge stores | Machine-extracted knowledge from indexed sources. LLM-curated with quality flags and confidence scores. |
| `validated` | `dewey promote` | Agent content promoted by human review. Middle trust. |
| `draft` | `store_learning`, `dewey compile` | Agent-generated content. Unreviewed. Default for learnings and compiled articles. |
| `untrusted` | source config | Content from sources explicitly marked as lower trust by the user. Lowest trust. |

Filter by tier: `semantic_search_filtered(query: "auth", tier: "authored")` returns only human-written content.
Filter by tier: `semantic_search_filtered(query: "auth", tier: "curated")` returns only knowledge store content.

### Content Sanitization

Sources can be configured with `trust_tier` and `sanitize_mode` fields in `sources.yaml`:

- **`trust_tier`**: One of `authored` (default), `curated`, `validated`, `draft`, or `untrusted`. Sets the trust tier for all pages from this source.
- **`sanitize_mode`**: One of `warn` (default for web/github), `strict`, or `off` (default for disk/code). Controls sanitization behavior:
  - `warn`: Scan content and merge findings into page properties. Content is still indexed.
  - `strict`: Scan content and reject documents with `critical` or `high` severity findings.
  - `off`: Skip sanitization entirely.

The `dewey doctor` command reports sanitization findings aggregated by source and severity. The `dewey lint` command surfaces individual pages with findings and flags stale pattern versions.

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
3. **Release** (`.github/workflows/release.yml`): Triggered via `workflow_dispatch` with a `tag` input (e.g., `gh workflow run release.yml -f tag=v1.2.3`). Delegates to `complytime/org-infra` reusable workflows (pinned by SHA) for preflight validation (branch, semver format, tag uniqueness with re-run resilience, semver ordering, CI check status via `build-and-test` + `MegaLinter`, unreleased commits) and GoReleaser execution with supply chain artifacts (cosign signatures, SBOMs). Optional skip inputs (`skip_semver_check`, `skip_ci_checks`, `skip_unreleased_check`) allow bypassing individual preflight checks for emergency releases. Builds cross-platform binaries (darwin/linux x amd64/arm64) and RPM packages, creates GitHub Releases, signs and notarizes macOS archives with Apple Developer ID, patches Homebrew cask checksums with signed values, and pushes to `unbound-force/homebrew-tap`.

## Sibling Repositories

| Repo | Purpose | Constitution | Status |
|------|---------|--------------|--------|
| `unbound-force/unbound-force` | Meta repo (specs, governance, CLI) | v1.1.0 (org constitution) | Active |
| `unbound-force/gaze` | Go static analysis (tester hero) | v1.3.0 (Accuracy, Minimal Assumptions, Actionable Output, Testability) | Active |
| `unbound-force/website` | Public website (Hugo + Doks) | v1.0.0 (Content Accuracy, Minimal Footprint, Visitor Clarity) | Active |
| `unbound-force/homebrew-tap` | Homebrew cask distribution | N/A | Active |

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
  017-openai-compatible-providers/ # OpenAI-compatible embedding and synthesis gateways, environment-only credentials (Complete)
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
- Go 1.25 (per `go.mod`) + `github.com/modelcontextprotocol/go-sdk` (MCP SDK), `github.com/unbound-force/dewey/v3/source` (source manager), `github.com/unbound-force/dewey/v3/store` (SQLite persistence), `github.com/unbound-force/dewey/v3/embed` (Ollama embeddings), `github.com/unbound-force/dewey/v3/vault` (document parsing/persistence), `sync` (mutex for mutual exclusion) (011-live-reindex)
- N/A (no storage changes — MCP tools wrap existing indexing pipeline, no new tables or schema changes) (011-live-reindex)
- Go 1.25 (per `go.mod`) + `sync/atomic` (readiness flag), `sync` (shared mutex), `github.com/modelcontextprotocol/go-sdk` (MCP SDK), `github.com/charmbracelet/log` (logging) (012-background-index)
- N/A (no storage changes — restructures startup sequence, no new tables or schema changes) (012-background-index)
- Go 1.25 (per `go.mod`) + `modernc.org/sqlite` (pure-Go SQLite, schema migration v1→v2), `github.com/modelcontextprotocol/go-sdk` (MCP SDK — 3 new tools: compile, lint, promote), `github.com/spf13/cobra` (CLI — 3 new commands), `github.com/charmbracelet/log` (logging), `github.com/unbound-force/dewey/v3/embed` (Ollama embeddings for clustering), `net/http` (Ollama /api/generate for LLM synthesis) (013-knowledge-compile)
- SQLite schema v1→v2: `pages` table gains `tier TEXT DEFAULT 'authored'` and `category TEXT` columns + `idx_pages_tier` index. New `llm/` package for LLM synthesis interface. (013-knowledge-compile)

- Go 1.25 (per `go.mod`) + `modernc.org/sqlite` v1.47.0 (pure-Go SQLite), `github.com/k3a/html2text` v1.4.0 (HTML-to-text for web crawl), `github.com/modelcontextprotocol/go-sdk` v1.2.0 (existing MCP SDK), `github.com/spf13/cobra` (CLI framework), `github.com/charmbracelet/log` (structured logging) (001-core-implementation)
- Go 1.25 (per `go.mod`) + `gopkg.in/yaml.v3` (knowledge store config parsing), `github.com/unbound-force/dewey/v3/llm` (LLM synthesis for curation), `github.com/unbound-force/dewey/v3/curate` (new package — config parsing + curation pipeline), `github.com/spf13/cobra` (CLI — `dewey curate` command), `github.com/modelcontextprotocol/go-sdk` (MCP SDK — `curate` tool) (015-curated-knowledge-stores)
- N/A (no schema changes — `curated` is a new value in the existing `tier TEXT` column. File-backed learnings use filesystem, not new tables) (015-curated-knowledge-stores)
- Go 1.25 (per `go.mod`) + stdlib `net/http`, `encoding/json`, `net/url`, `math/rand/v2`, `sync`, `time` (OpenAI-compatible transport; no vendor SDK), `github.com/charmbracelet/log` (logging) (017-openai-compatible-providers)
- N/A (no schema changes — vectors and completions flow through existing storage and tool paths) (017-openai-compatible-providers)
- Go 1.25 (per `go.mod`) + stdlib git plumbing via `os/exec` (worktrees, stashes, commit surgery), `github.com/unbound-force/dewey/v3/sanitize` (credential screening), `github.com/spf13/cobra` (CLI — `dewey artifacts plan|sync` commands) (dewey-publish)
- Go 1.25 (per `go.mod`) + `net/http` (OpenAI-compatible chat completions), `github.com/modelcontextprotocol/go-sdk` (MCP tools), `github.com/unbound-force/dewey/v3/llm` (provider factory), `modernc.org/sqlite` (page resolution), `github.com/spf13/cobra` (CLI — multi-page `dewey promote`) (dewey-distill)

## Recent Changes
- curator-agent-human-gates (OpenSpec, dewey #152): Added a least-privilege `curator` agent (`.opencode/agents/curator.md`, scaffolded by `dewey init` when `.opencode/` exists, tracked/shipped byte-identical via `agents.go`), bound `/dewey-distill` to it via `agent:` frontmatter, and made its gates human-only. New `dewey gate auto-answer` subcommand (`gate.go`) deterministically resolves the opencode-auto-question plugin config (JSONC with full-entry `{env:VAR}` interpolation, trim + case-fold normalization, documented default `["autonomy"]`), scans plugin registration as advisory context, and fails closed on unreadable/unparseable/unresolvable config or a missing curator definition; both gate questions carry the `HUMAN_GATE:` marker. No guard relaxation is committed to the repo — any curator supervisor override lives in the user's global config. Website: docs website#371, tutorial website#376, blog website#377.
- read-tools-bare-names (OpenSpec): `get_page` and every name-based read through the vault client (including `get_links` forward direction, `traverse` start pages, `get_whiteboard`, `bulk_update_properties`, and `journal_range` — illustrative, since the seam is every `GetPage`/`GetPageBlocksTree` caller) now resolve bare names on the store fallback — exact match, then `compiled/<name>`, then `learning/<name>`, first match wins; vault files keep precedence, slash-bearing names skip the shorthand, and the write guard plus `promote` stay exact/strict. Per-probe store lookups are wrapped with the key they attempted (dewey #140)
- serialize-supersession-writes (OpenSpec, dewey #166): concurrent same-target supersessions in `store_learning` are serialized at write time — a compare-and-swap store claim (`Store.ClaimSupersession`, guarded on the target recording no `superseded_by`) plus a file leg that re-reads the markdown backup and refuses to overwrite a differing recorded successor, so exactly one successor is recorded; a losing call keeps its learning with its `supersedes` claim and returns `supersedes_warning` naming the winner; supersession and lint's anchor fix share one `withLearningPropsLock` hold across both legs; the winning chain survives `graph.db` loss through re-ingestion.
- lint-anchor-drift (OpenSpec, dewey #147 / PR #159): `lint` gained `anchor_missing` and `anchor_drift` checks for learnings whose `as_of` pins the vault's own repository, comparing each `<repo>:<path>` evidence locator at the pin and `HEAD` via local git (`tools/lint_anchors.go`, `anchorChecker` interface; 5s per call, 60s per pass, request context; `--no-ext-diff --no-textconv`). Every result carries `anchor_checks` (`checked`/`skipped`/`failed` + `reason`). `fix` marks drifted learnings `validity: stale-unverified` in store and backup, never tiers; `fixed.anchor_drift` counts store writes and `anchor_fix_warnings` lists backup failures. Backup writes and property decoding are shared with #146 supersession (`updateLearningBackup`, `decodeLearningProperties`). Website: docs website#398, blog website#399.
- search-compound-terms (OpenSpec): In-memory search index now indexes hyphenated and snake_case compound terms whole AND by component (≥2 bytes, matching the tokenize term-length convention); queries return exact-term matches first, then fill remaining slots up to the limit with blocks matching every component; the FullTextSearch store merge is gated on the exact-match count so store-only pages holding a full compound ID rank ahead of component-only fills (resolves #149, part of #153)
- learning-temporal-validity (OpenSpec, dewey #146 / PR #156): `store_learning` gained temporal-validity metadata — automatic `as_of` git pin (`<repo>@<HEAD sha>`, injectable via `WithAsOfResolver`), optional `evidence` (≤50 locators, 512 runes each), `validity` (`current`/`stale-unverified`/`superseded`/`historical`, absent = unknown), and two-way `supersedes`/`superseded_by` links (append-only, `supersedes_warning` on partial back-link failure); new `tools/learning_temporal.go` pure helpers; all validation runs before any write; fields round-trip through re-ingestion; no SQL schema change (fields live in `properties` JSON)
- 017-openai-compatible-providers: Added `openaicompat/` shared transport, `OpenAICompatibleEmbedder`, `OpenAICompatibleSynthesizer`, environment-only API-key chain (`DEWEY_EMBEDDING_API_KEY` / `DEWEY_SYNTHESIS_API_KEY` → `DEWEY_API_KEY` → `OPENROUTER_API_KEY` on the exact `openrouter.ai` host), 429 retry parity with Vertex, cached `/models` probe, provider-aware doctor/startup/index warnings, and `UnavailableReasoner` capability interfaces on both providers
- dewey-distill (OpenSpec): Added the `openai-compatible` synthesis provider (`synthesis.api_key`/`DEWEY_SYNTHESIS_API_KEY`, configurable timeout and max tokens, 429 retry, truncation detection), server-side `compile`/`curate` wiring via `WithSynthesizer`, shared index mutex for compile, validated-article retention through compiles plus `store_compiled` validated-overwrite refusal, batch `promote` with a tools-side resolution ladder (`resolvePromotePage`) over index-backed store queries backed by schema v3 NOCASE indexes (subsumes #91), `/dewey-distill` slash command, and doctor diagnostics for the new provider
- mcp-internal-store-access (OpenSpec): MCP read tools (get_page, get_page_blocks_tree, get_block, list_pages, search, find_by_tag) now serve a unified live view of vault files and persistent store pages via vault read bridge (RLock snapshot → live store read-through, vault precedence, no caching); promote resolves bare names against compiled/ then learning/ and distinguishes lookup errors from absence; CLI promote reports lock-holder PID/command; shared persistent-storage error with lock hint across tools (dewey #90, #91)
- dewey-publish (OpenSpec): Added `artifacts` package (plan/sync sweep of `.uf/dewey/{learnings,compiled,knowledge}` from working tree, stashes, unpushed commits; credential screening; bounded gitignore fix), `dewey artifacts plan|sync` CLI commands, `/dewey-publish` slash command (non-destructive, mandatory human approval gate)
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

## Convention Packs

This repository uses convention packs scaffolded by
unbound-force. Agents MUST read the applicable pack(s)
before writing or reviewing code.

- `.opencode/uf/packs/default.md`
- `.opencode/uf/packs/default-custom.md`
- `.opencode/uf/packs/severity.md`
- `.opencode/uf/packs/content.md`
- `.opencode/uf/packs/content-custom.md`
- `.opencode/uf/packs/go.md`
- `.opencode/uf/packs/go-custom.md`
