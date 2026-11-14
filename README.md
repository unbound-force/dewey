# Dewey

Knowledge graph MCP server that gives AI full access to your knowledge graph. Supports **Logseq** and **Obsidian** — both with full read-write support. Navigate pages, search blocks, analyze link structure, track decisions, manage flashcards, compile knowledge, curate knowledge stores, and write content — all through the [Model Context Protocol](https://modelcontextprotocol.io).

Hard fork of [graphthulhu](https://github.com/skridlevsky/graphthulhu) by Max Skridlevsky, extended with persistent SQLite storage, vector-based semantic search via Ollama, pluggable LLM providers (Ollama, Vertex AI, and any OpenAI-compatible gateway), pluggable content sources (disk, GitHub, web crawl, code), knowledge compilation with temporal intelligence, curated knowledge stores, and trust tiers for the [Unbound Force](https://github.com/unbound-force) AI agent swarm ecosystem.

Built in Go with the [official MCP Go SDK](https://github.com/modelcontextprotocol/go-sdk).

## Why

Your knowledge graph stores interconnected pages, blocks, and links. But AI assistants can't see any of it — they're blind to your second brain.

Dewey fixes that. It exposes your entire knowledge graph through MCP, so any MCP client can:

- Read any page with its full block tree, parsed links, tags, and properties
- Search across all blocks with contextual results (parent chain + siblings)
- Traverse the link graph to discover how concepts connect
- Find knowledge gaps — orphan pages, dead ends, weakly-linked areas
- Discover topic clusters through connected component analysis
- Create pages, write blocks, build hierarchies, link pages bidirectionally
- Query with raw DataScript/Datalog for anything the built-in tools don't cover (Logseq)
- Review flashcards with spaced repetition statistics (Logseq)
- Explore whiteboards and their spatial connections (Logseq)

It turns "tell me about X" into an AI that actually understands your knowledge graph's structure.

## Tools

52 tools across 16 categories. Most work with both backends; some are Logseq-only (DataScript queries, flashcards, whiteboards).

Read tools (`get_page`, `get_block`, `list_pages`, `search`, `find_by_tag`) return a unified live view of vault files and internal store pages (learnings, compiled articles) — pages stored this session via `store_learning` or `store_compiled` are visible immediately, with no restart. Vault files take precedence over same-named store pages. Read store-backed pages by their canonical names (`compiled/<tag>` for compiled articles, `learning/<identity>` for learnings) or by the bare name: `list_pages` shows compiled articles under their bare topic, and `get_page` tries the exact name, then `compiled/<name>`, then `learning/<name>`. Every name-based read through the vault client — including `get_links` forward direction, `traverse` start pages, `get_whiteboard`, `bulk_update_properties`, and `journal_range`, whose journal-format page names are slash-free — shares the resolution. The blast radius is every `GetPage`/`GetPageBlocksTree` caller, so the list is illustrative, not exhaustive. Name-based tools that resolve through other paths — `similar` and `find_connections` — still need the canonical name. `promote` accepts either form, but reports a name that matches both namespaces as ambiguous, while reads serve the compiled article. On store-backed pages, `find_by_tag` matches frontmatter `tag`/`tags` properties, block `#tags`, and the `topic` property of compiled articles.

### Navigate

| Tool | Backend | Description |
|------|---------|-------------|
| `get_page` | Both | Full recursive block tree with parsed links, tags, properties; a bare name also finds `compiled/<name>` and `learning/<name>` |
| `get_block` | Both | Block by UUID with ancestor chain, children, siblings |
| `list_pages` | Both | Filter by namespace, property, or tag; sort by name/modified/created |
| `get_links` | Both | Forward and backward links with the blocks that contain them |
| `get_references` | Logseq | All blocks referencing a specific block via `((uuid))` |
| `traverse` | Both | BFS path-finding between two pages through the link graph |

### Search

| Tool | Backend | Description |
|------|---------|-------------|
| `search` | Both | Full-text search with parent chain + sibling context; on the vault in-memory index, hyphenated and snake_case terms also match by component (`KB-REVIEW-ADJUD`, `KB REVIEW ADJUD 01`), after exact matches |
| `query_properties` | Both | Find by property values with operators (eq, contains, gt, lt) |
| `query_datalog` | Logseq | Raw DataScript/Datalog queries against the Logseq database |
| `find_by_tag` | Both | Tag search with child tag hierarchy support |

### Analyze

| Tool | Backend | Description |
|------|---------|-------------|
| `graph_overview` | Both | Global stats: pages, blocks, links, most connected, namespaces |
| `find_connections` | Both | Direct links, shortest paths, shared connections between pages |
| `knowledge_gaps` | Both | Orphan pages, dead ends, weakly-linked areas |
| `list_orphans` | Both | List orphan page names with block counts and property status |
| `topic_clusters` | Both | Connected components with hub identification |
| `health` | Both | Check server status: version, backend, read-only mode, page count, embedding status, sources |

### Write

| Tool | Backend | Description |
|------|---------|-------------|
| `create_page` | Both | New page with properties and initial blocks |
| `append_blocks` | Both | Append plain-text blocks (simpler than upsert_blocks) |
| `upsert_blocks` | Both | Batch create with nested children for deep hierarchies |
| `update_block` | Both | Replace block content by UUID |
| `delete_block` | Both | Remove block and all children |
| `move_block` | Both | Reposition before, after, or as child of another block (cross-page supported) |
| `link_pages` | Both | Bidirectional link with optional relationship context |
| `delete_page` | Both | Remove a page and all its blocks |
| `rename_page` | Both | Rename page and update all `[[links]]` across the graph |
| `bulk_update_properties` | Both | Set a property on multiple pages in one call |

### Decision

| Tool | Backend | Description |
|------|---------|-------------|
| `decision_check` | Both | Surface open, overdue, and resolved decisions with deadline status |
| `decision_create` | Both | Create a DECIDE block with `#decision` tag, deadline, options, context |
| `decision_resolve` | Both | Mark a decision as DONE with resolution date and outcome |
| `decision_defer` | Both | Push deadline with reason, tracks deferral count, warns after 3+ |
| `analysis_health` | Both | Audit analysis/strategy pages for graph connectivity (3+ links or has decision) |

### Journal

| Tool | Backend | Description |
|------|---------|-------------|
| `journal_range` | Both | Entries across a date range with full block trees |
| `journal_search` | Both | Search within journals, optionally filtered by date |

### Flashcard

| Tool | Backend | Description |
|------|---------|-------------|
| `flashcard_overview` | Logseq | SRS stats: total, due, new vs reviewed, average repeats |
| `flashcard_due` | Logseq | Cards due for review with ease factor and interval |
| `flashcard_create` | Logseq | Create front/back card with `#card` tag |

### Whiteboard

| Tool | Backend | Description |
|------|---------|-------------|
| `list_whiteboards` | Logseq | All whiteboards in the graph |
| `get_whiteboard` | Logseq | Embedded pages, block references, visual connections |

### Semantic Search

| Tool | Backend | Description |
|------|---------|-------------|
| `semantic_search` | Both | Find documents semantically similar to a natural language query. Results carry `validity` / `as_of`; superseded (×0.6) and stale-unverified (×0.85) results rank lower unless `ignore_validity: true` (reported similarity stays raw) |
| `similar` | Both | Find the most similar documents to a given page or block. Results carry `validity` / `as_of` when the page has them (enrichment only — no validity ranking, filtering, or over-fetch) |
| `semantic_search_filtered` | Both | Semantic search with metadata filters (source, properties, tier, validity), with the same validity ranking as `semantic_search` |

### Compile

| Tool | Backend | Description |
|------|---------|-------------|
| `compile` | Both | Synthesize stored learnings into compiled knowledge articles |
| `store_compiled` | Both | Persist a compiled article synthesized by the calling agent |

### Lint

| Tool | Backend | Description |
|------|---------|-------------|
| `lint` | Both | Scan knowledge base for quality issues (stale decisions, embedding gaps, knowledge store quality, anchor drift and missing anchors on pinned learnings) |

### Promote

| Tool | Backend | Description |
|------|---------|-------------|
| `promote` | Both | Promote draft content to validated after human review; names resolve exact, then case-insensitive, then bare-tag shorthand (`name` tries `compiled/name`, `learning/name`, and `knowledge/*` pages), then source doc IDs; ambiguous matches are refused; batch promotion is all-or-nothing |

### Verify Candidates

| Tool | Backend | Description |
|------|---------|-------------|
| `verify_candidates` | Both | Read-only verification of the promotable candidate queue (draft pages under `learning/`, `compiled/`, and `knowledge/`) ahead of the human promotion gate; checks each candidate's evidence locators against its `as_of` pin and `HEAD` via local git and reports `anchors_ok`, `drifted`, `missing`, `no_anchors`, or `skipped` per candidate with summary counts; compiled articles aggregate their sources; nothing is mutated — verification is not endorsement, and promotion stays a human decision |

### Indexing

| Tool | Backend | Description |
|------|---------|-------------|
| `index` | Both | Fetch and index configured content sources |
| `reindex` | Both | Delete and rebuild all external source content |
| `reload` | Obsidian | Force a full vault re-index from disk; store-backed pages (learnings, compiled articles) are re-loaded from the persistent store, not dropped |

### Learning

| Tool | Backend | Description |
|------|---------|-------------|
| `store_learning` | Both | Store a learning with tag, category, and temporal identity; pins `as_of` and accepts `evidence`, `validity`, and `supersedes` ([Temporal Validity](#temporal-validity)) |
| `supersede_learning` | Both | Record a confirmed supersession between two existing learnings — `successor` supersedes `target` — after full validation: distinct identities, existence, no recorded successor on the target, no conflicting `supersedes` claim, and a bounded cycle walk in both directions. Writes the two-way link in the store and markdown backups; partial failures surface as `supersedes_warning` ([Temporal Validity](#temporal-validity)) |

### Curate

| Tool | Backend | Description |
|------|---------|-------------|
| `curate` | Both | Run the curation pipeline to extract structured knowledge from indexed sources into knowledge stores |

## Install

### Homebrew (macOS)

```bash
brew install --cask unbound-force/tap/dewey
```

The cask includes a signed and notarized binary. Ollama is installed automatically as a dependency.

### RPM (Fedora, RHEL, CentOS)

Download the `.rpm` package from the [Releases page](https://github.com/unbound-force/dewey/releases) and install with `dnf`:

```bash
sudo dnf install ./dewey_<version>_linux_amd64.rpm
```

RPM packages are available for both `amd64` and `arm64` architectures. The binary installs to `/usr/bin/dewey`.

### go install (all platforms)

```bash
go install github.com/unbound-force/dewey/v3@latest
```

Requires Go 1.25+. Works on macOS, Linux, and any platform with a Go toolchain.

### Build from source

```bash
git clone https://github.com/unbound-force/dewey.git
cd dewey
go build -o dewey .
```

### Setup: Logseq

1. In Logseq, go to **Settings → Features** and enable **HTTP APIs server**
2. Click the **API** icon that appears in the top toolbar
3. Click **Start Server**
4. Click **Create Token** and copy the generated token — you'll need it for configuration

The API runs on `http://127.0.0.1:12315` by default.

### Setup: Obsidian

No plugins or server required. Dewey reads your vault's `.md` files directly.

You need to provide the path to your vault:

```bash
dewey serve --backend obsidian --vault /path/to/your/vault
```

Or via environment variables:

```bash
export DEWEY_BACKEND=obsidian
export OBSIDIAN_VAULT_PATH=/path/to/your/vault
dewey
```

The Obsidian backend supports full read-write operations. It parses YAML frontmatter into properties, builds a block tree from headings, and indexes `[[wikilinks]]` for backlink resolution. Writes use atomic temp-file renames, and the in-memory index is rebuilt after every mutation. File watching (fsnotify) keeps the index in sync with external edits. Daily notes are detected from a configurable subfolder (default: `daily notes`).

## Configuration

### Logseq — OpenCode

Add to your `opencode.json`:

```json
{
  "mcp": {
    "dewey": {
      "type": "local",
      "command": ["dewey"],
      "env": {
        "LOGSEQ_API_URL": "http://127.0.0.1:12315",
        "LOGSEQ_API_TOKEN": "your-token-here"
      }
    }
  }
}
```

### Obsidian — OpenCode

```json
{
  "mcp": {
    "dewey": {
      "type": "local",
      "command": ["dewey", "--backend", "obsidian", "--vault", "/path/to/your/vault"]
    }
  }
}
```

### Read-only mode

To disable all write operations (Obsidian backend, the default):

```json
{
  "mcp": {
    "dewey": {
      "type": "local",
      "command": ["dewey", "--read-only", "--backend", "obsidian", "--vault", "/path/to/your/vault"]
    }
  }
}
```

For the Logseq backend (requires Logseq running with API enabled):

```json
{
  "mcp": {
    "dewey": {
      "type": "local",
      "command": ["dewey", "--read-only", "--backend", "logseq"],
      "env": {
        "LOGSEQ_API_URL": "http://127.0.0.1:12315",
        "LOGSEQ_API_TOKEN": "your-token-here"
      }
    }
  }
}
```

### Version control warning

On startup with the Logseq backend, Dewey checks if your graph directory is git-controlled. If not, it prints a warning to stderr suggesting you initialize version control. Write operations cannot be undone without it.

### Persistence

Dewey stores its index in `.uf/dewey/graph.db` (SQLite). The database holds pages, blocks, links, vector embeddings, and source metadata. After the first full index, subsequent sessions load from the persistent index and only reprocess changed files — startup is near-instant.

Run `dewey init` to create the `.uf/dewey/` directory with default configuration:

```bash
dewey init
```

This creates:
- `.uf/dewey/config.yaml` — embedding model and endpoint settings
- `.uf/dewey/sources.yaml` — content source configuration (empty by default)
- `.uf/dewey/graph.db` — created automatically on first `dewey serve` or `dewey index`
- `.uf/dewey/dewey.log` — created automatically by `dewey serve` for MCP server diagnostics (truncated at 10 MB on startup)
- `.opencode/agents/curator.md` — the least-privilege curator agent for `/dewey-distill` (created only when `.opencode/` already exists; an existing file is never overwritten)

Ignore runtime state, not knowledge. The machine-local runtime state — `.uf/dewey/graph.db` (with its `-shm` and `-wal` sidecars), `dewey.log`, `dewey.lock`, and `cache/` — is rebuilt from source files and belongs in your `.gitignore`; the private configuration files (`config.yaml`, `sources.yaml`, `knowledge-stores.yaml`) should stay out of git too. The knowledge artifact directories (`.uf/dewey/learnings/`, `compiled/`, `knowledge/`) are **not** rebuildable: do not ignore them. Publish them with `/dewey-publish` or `dewey artifacts` instead.

### Environment variables

| Variable | Default | Description |
|----------|---------|-------------|
| `LOGSEQ_API_URL` | `http://127.0.0.1:12315` | Logseq HTTP API endpoint |
| `LOGSEQ_API_TOKEN` | (required for Logseq) | Bearer token from Logseq settings |
| `DEWEY_BACKEND` | `obsidian` | Backend type: `obsidian` (default) or `logseq` |
| `OBSIDIAN_VAULT_PATH` | — | Path to Obsidian vault root |
| `DEWEY_EMBEDDING_MODEL` | `granite-embedding:30m` | Embedding model name |
| `DEWEY_EMBEDDING_ENDPOINT` | `http://localhost:11434` | Embedding API endpoint (required for `openai-compatible`, no Ollama fallback) |
| `DEWEY_CHUNK_MAX_CHARS` | `12288` | Max chunk characters for embedding (model-dependent) |
| `DEWEY_EMBEDDING_API_KEY` / `DEWEY_API_KEY` | *(none)* | OpenAI-compatible embedding gateway credentials (environment-only) |
| `DEWEY_SYNTHESIS_API_KEY` | *(none)* | OpenAI-compatible synthesis key (wins over `synthesis.api_key`) |
| `DEWEY_AUTHOR` | — | Override author identity for `store_learning` (useful in CI or shared environments) |
| `GITHUB_TOKEN` / `GH_TOKEN` | — | GitHub API token for content sources |

### Global flags

| Flag | Short | Description |
|------|-------|-------------|
| `--verbose` | `-v` | Enable debug logging (shows UUID seeds, block insertions, lock detection) |
| `--log-file PATH` | | Write logs to file in addition to stderr |
| `--no-embeddings` | | Skip embedding generation (on serve, index, reindex) |
| `--vault PATH` | | Path to vault (on serve, index, reindex, status, search, doctor, manifest, artifacts plan, artifacts sync) |

When running as an MCP server (`dewey serve`), Dewey automatically logs to `.uf/dewey/dewey.log` for diagnostics. The log file is truncated when it exceeds 10 MB.

## CLI Commands

### dewey init

Initialize a Dewey configuration in the current directory. Creates `.uf/dewey/` with default `config.yaml` and `sources.yaml`.

```bash
dewey init [--vault PATH]
```

When `.opencode/` already exists, `dewey init` also scaffolds `.opencode/agents/curator.md` (the `/dewey-distill` agent) and the Dewey slash commands. Scaffolding is idempotent: an existing file is never overwritten, so a stale scaffolded agent must be updated deliberately.

### dewey index

Build or update the knowledge graph and embedding indexes from all configured sources. Uses content hashing for incremental updates — only re-indexes changed files.

```bash
dewey index [--vault PATH] [--source NAME] [--force] [--no-embeddings]
```

- `--force` — Re-fetch all sources, even if within their refresh interval
- `--no-embeddings` — Skip embedding generation (keyword search still works)

### dewey reindex

Delete the existing index and rebuild from scratch. Use when upgrading Dewey, recovering from corruption, or after fixing UUID collision issues.

```bash
dewey reindex [--vault PATH] [--no-embeddings]
```

This removes `graph.db`, WAL/SHM files, and the lock file before rebuilding. Requires stopping `dewey serve` first (the lock file prevents concurrent access).

### dewey status

Report index health: page count, block count, embedding coverage, source status.

```bash
dewey status [--vault PATH] [--json]
```

### dewey doctor

Run diagnostic checks for Dewey dependencies and report pass/fail with fix instructions. Checks: workspace initialization, database health (per-source page counts), embedding layer configuration (Ollama availability and model status; Vertex checked for configuration completeness; OpenAI-compatible checked for configuration, connectivity, and credentials), synthesis layer configuration (Ollama/Vertex/OpenAI-compatible provider, model and credential presence — only Ollama availability is probed; Vertex and OpenAI-compatible gateways are checked for configuration completeness), MCP server process, and opencode.json configuration.

```bash
dewey doctor [--vault PATH]
```

### dewey search

Full-text search across the knowledge graph (local files and external sources from graph.db).

```bash
dewey search [--vault PATH] [--limit N] QUERY
```

### dewey source add

Add a content source (GitHub or web) to the configuration.

```bash
dewey source add github --org ORG --repos REPO1,REPO2 [--refresh INTERVAL]
dewey source add web --url URL [--name NAME] [--depth N] [--refresh INTERVAL]
```

### dewey manifest

Generate `.uf/dewey/manifest.md` — a structured summary of CLI commands, MCP tools, and exported packages discovered via AST parsing of Go source files.

```bash
dewey manifest [--vault PATH]
```

### dewey journal

Append a block to a Logseq journal page.

```bash
dewey journal [--vault PATH] CONTENT
```

### dewey add

Append a block to a named Logseq page.

```bash
dewey add [--vault PATH] PAGE CONTENT
```

### dewey compile

Synthesize stored learnings into compiled knowledge articles. Reads all learnings, clusters them by topic tag, and produces current-state articles that cite their source learnings per claim. Supersession is explicit, never recency-driven: synthesis presents unresolved contradictions with both positions, proposes supersessions for human confirmation (`proposed_supersessions`), and reports per-article citation `coverage`.

```bash
dewey compile [--vault PATH]
```

### dewey lint

Scan the knowledge base for quality issues: stale decisions, embedding gaps, contradictions, orphaned content, and citation coverage.

```bash
dewey lint [--vault PATH] [--fix]
```

**Anchor checks.** For learnings pinned with `as_of` to the vault's own repository (see [Temporal Validity](#temporal-validity)), lint checks each `<repo>:<path>` evidence locator for that repository against local git:

- `anchor_missing`: the path does not exist at the `as_of` commit, or that commit is not in the repository (one finding per learning).
- `anchor_drift`: the path changed, or was removed, between the `as_of` commit and `HEAD`. Paths are compared literally, so a file name containing glob characters matches only that file.

`--fix` marks drifted learnings `validity: stale-unverified` (store and markdown file); tiers are never changed, and a learning superseded in the meantime keeps `superseded`. `fixed.anchor_drift` counts learnings marked in the store; a markdown file that could not be updated is listed in `anchor_fix_warnings`. Every result reports `anchor_checks` (`checked`, `skipped`, or `failed`, with a `reason`), so a skipped pass is never mistaken for a clean one. Skipped outside a git repository. Not checked: `<repo>#<n>` locators (no network), other repositories, learnings pinned to another repository, learnings whose evidence exceeds `store_learning`'s limits, and learnings marked `superseded` or `historical`. The 30-day `stale_decision` check remains as a secondary signal. The same anchor checks back `/dewey-distill`'s verify step — the `verify_candidates` MCP tool reports them per candidate at the review gate.

**Citation coverage.** Compiled articles backed by the store are checked in both directions: citations naming learnings absent from the article's declared `sources` (`unresolved`), and current declared sources the article never cites (`uncited`). Findings carry the `citation_coverage` kind and name each violation by identity; `--fix` never touches them — the remediation is to recompile the article or correct its citations. The store is the check's data source, so a file-only hand edit to `.uf/dewey/compiled/` is invisible until the article is recompiled or re-indexed; an article written before this check existed is reported as intended signal, not a false positive.

### dewey promote

Promote a page from `draft` tier to `validated` after human review. Only draft-tier pages (agent-generated content) can be promoted.

```bash
dewey promote [--vault PATH] PAGE_NAME...
```

The page is located by its exact name; when that name is absent, resolution falls back deterministically: a case-insensitive exact match, then — for a bare name containing no `/` — the `compiled/PAGE_NAME`, `learning/PAGE_NAME`, and `knowledge/*/PAGE_NAME` namespaces, followed by a `source_id`/`source_doc_id` match. A bare tag that matches more than one page reports the candidates instead of picking one. A missing page reports `Page '<name>' not found`; a store failure reports `failed to look up page '<name>': <error>` so the error is not mistaken for absence.

When the store cannot be opened because another Dewey process holds the database lock, the error identifies the lock holder (PID and command, read from `.uf/dewey/dewey.lock`) and points to the `dewey_promote` MCP tool as the supported write path while a server is running.

Multiple names are promoted as an all-or-nothing batch: if any page is missing, ambiguous, or not `draft`, no tiers change and the command reports each failure.

### dewey curate

Run the curation pipeline to extract structured knowledge from indexed sources into knowledge stores. Uses the configured synthesis provider (Ollama, Vertex AI, or OpenAI-compatible) to analyze indexed content and produce curated markdown files with confidence scoring and quality flags.

```bash
dewey curate [--vault PATH] [--store NAME] [--force] [--no-embeddings]
```

- `--store` — Curate a specific named store (default: all configured stores)
- `--force` — Run full curation instead of incremental (re-process all documents)
- `--no-embeddings` — Skip embedding generation for curated files

### dewey artifacts

Plan and publish stranded knowledge artifacts — learnings, compiled articles, and knowledge-store output under `.uf/dewey/` — into a git branch behind a mandatory human approval gate. The plan sweeps the working tree, git stashes, and unpushed commits, deduplicates identical variants, screens for credential-like content, and reports a bounded `.gitignore` fix. Sync materializes an approved plan in a dedicated worktree, commits artifacts in tag groups, and renders a PR body. The vault working tree is never touched — the sweep is non-destructive and only reads what git already tracks.

```bash
dewey artifacts plan [--json] [--vault PATH]
dewey artifacts sync --plan FILE --approve [--approved-by ID] [--resolutions FILE] [--vault PATH]
```

- `--json` — Emit the full plan JSON to stdout (the plan file is always written to a `0600` temp file, with its path printed first)
- `--plan` — Path to the plan file to execute (required)
- `--approve` — The human gate: sync refuses to run without it
- `--approved-by` — Approver identity recorded in the commit trailer (default: `git config user.name`)
- `--resolutions` — Path-keyed JSON choosing among clean variants for conflicting artifacts

The plan JSON is a stable scripting surface for the `/dewey-publish` flow. Top-level keys: `schema_version`; `repo` (`base_branch`, `base_sha`, `publish_remote`, `push_remote`, `artifact_roots`, `stash_refs`); `gitignore` (a `status` plus an optional bounded `fix`, present only when the status is `bare-dewey-fix`); `artifacts[]` (`path`, `identity`, `tag`, `kind`, `sha256`, `sources`, `status`, `findings`); `conflicts[]` (`path`, `identity`, `variants[]` with `sha256`, `sources`, `findings`); `blocked[]` (`path`, `identity`, `findings` of `scanner`, `pattern`, `severity`, `line`, `message`); `warnings[]`; `summary`; and `rendered_summary` (rendered for humans — relay it verbatim). Artifact sources are labeled `working-tree`, `stash:stash@{N}`, `stash-untracked:stash@{N}`, or `commit:<sha>`; `kind` is one of `learning`, `compiled`, or `knowledge`. The summary is arithmetic over the arrays: `total` is the number of artifacts, `new` + `unchanged` equals it, `conflicts` counts the conflicts array, `blocked` counts the blocked array. Sync emits `branch`, `worktree`, `commit`, `files_written`, `gitignore_fix_applied`, `nothing_to_publish`, `pr_body`, `approved_by`, and `plan_sha256`.

Credential screening is a pre-exposure floor, not a guarantee. The scanner's rules — private-key blocks and credential-bearing connection strings at critical severity; `sk-`, `ghp_`, `xoxb_`, and `AKIA` prefixes at high severity — are heuristics. Secret scanners such as gitleaks run only after PR creation and provide the second line of defense once content leaves the machine.

When the plan reports gitignore status `bare-dewey-fix`, sync commits a bounded fix that re-ignores runtime state under `.uf/dewey/`-prefixed rules while leaving the artifact directories stageable. After the chore PR merges, artifact files become committable while runtime state stays ignored.

Sync materializes the plan in a dedicated worktree (its path is the sync result's `worktree` field) and leaves the chore branch checked out there. Once the PR is created, remove it with `git worktree remove <worktree>`; the chore branch itself stays for the PR.

### dewey gate

Deterministic startup gates consumed by Dewey commands.

```bash
dewey gate auto-answer
```

`auto-answer` evaluates whether the opencode-auto-question plugin is configured to auto-answer questions for the `curator` agent. It checks every path the plugin could read its config from (`auto-question.jsonc`, JSONC with `{env:VAR}` interpolation): the home-directory file (`$HOME/.config/opencode/auto-question.jsonc`) always, plus the XDG file (`$XDG_CONFIG_HOME/opencode/auto-question.jsonc`) when `XDG_CONFIG_HOME` is set to a non-empty absolute path other than `$HOME/.config` — when `XDG_CONFIG_HOME` is unset, empty, or yields the same directory, exactly one file is checked. The home-directory file is authoritative for the effective `agents` list and the default-applied flag (defaulting to `["autonomy"]` when the file or its `agents` key is absent), because the plugin reads only that path; every checked file is a refusal input — an offending or unverifiable file at any checked path refuses, and no clean file masks an offending one. It also verifies that `.opencode/agents/curator.md` exists and declares `mode: subagent`. Exit 0 means pass; exit 1 means refusal. It fails closed: a home directory that cannot be resolved (unset, empty, or relative `$HOME`, even when `XDG_CONFIG_HOME` is set), a relative `XDG_CONFIG_HOME`, an unreadable, unparseable, or unresolvable configuration at any checked path, a `curator` or `*` entry, or a missing curator definition all refuse. The verdict is tool-rendered, names every checked file on pass and refusal, and is meant to be relayed verbatim:

```
Auto-answer gate: PASS
Config file: /home/me/.config/opencode/auto-question.jsonc
Also checked: /home/me/.xdg-config/opencode/auto-question.jsonc
Effective agents: ["autonomy"]
Default applied: no
Curator definition: resolved
Registration: registered
```

One `Also checked:` line appears per additional path, on pass and refusal alike; with `XDG_CONFIG_HOME` unset, empty, or coinciding with `$HOME/.config`, there is no additional path and the output is unchanged.

`/dewey-distill` runs this gate before any pipeline stage and stops on any non-pass verdict.

## Content Sources

Dewey indexes content from four pluggable source types. Configure them in `.uf/dewey/sources.yaml`:

```yaml
sources:
  - name: local-vault
    type: disk
    config:
      path: .

  - name: org-repos
    type: github
    config:
      org: your-org
      repos:
        - repo-one
        - repo-two
      content_types:
        - issues
        - pulls
        - readmes
    refresh: daily

  - name: go-docs
    type: web
    config:
      urls:
        - https://pkg.go.dev/github.com/spf13/cobra
      depth: 2
    refresh: weekly
```

You can also add sources via the CLI instead of editing YAML directly:

```bash
dewey source add github --org your-org --repos repo-one,repo-two
dewey source add web --url https://pkg.go.dev/github.com/spf13/cobra --depth 2
```

After adding sources, build the index:

```bash
dewey index             # incremental — only fetches sources past their refresh interval
dewey index --force     # full rebuild — re-fetches everything
```

### Code Source

The `code` source type indexes source code files using language-aware AST chunking. Currently supports Go, with a pluggable architecture for additional languages.

```yaml
  - name: project-code
    type: code
    config:
      path: "../replicator"
      languages:
        - go
```

The Go chunker extracts: package doc comments, exported function/method signatures, type declarations, Cobra CLI commands, and MCP tool registrations. Test files are excluded by default.

### Ignore Support

Dewey respects `.gitignore` files at the root of each source directory. Additionally, disk and code sources support `ignore` and `recursive` fields in `sources.yaml`:

```yaml
  - name: disk-website
    type: disk
    config:
      path: "../website"
      ignore: [drafts, temp]     # additional patterns beyond .gitignore
      recursive: true            # set false for top-level files only
```

## Semantic Search Setup

Semantic search requires an embedding provider. Dewey defaults to [Ollama](https://ollama.ai) running locally, but also supports [Google Vertex AI](#vertex-ai-provider) and any [OpenAI-compatible gateway](#openai-compatible-gateway-provider). All keyword-based tools work without an embedding provider — only the 3 semantic search tools (`semantic_search`, `similar`, `semantic_search_filtered`) require one.

### Ollama (Default)

Dewey auto-starts Ollama if it's installed but not running. No manual `ollama serve` needed — Dewey detects Ollama's state (External/Managed/Unavailable) and starts a subprocess if necessary. The subprocess is detached so it outlives Dewey.

```bash
brew install --cask ollama-app  # macOS — or download from https://ollama.ai
ollama pull granite-embedding:30m   # IBM Granite, 63 MB, Apache 2.0
```

### Verify

```bash
dewey status
```

The output shows embedding coverage. If Ollama is running and the model is pulled, you'll see embedding stats. If the provider is unavailable, Dewey logs a warning at startup and disables semantic search — all other tools continue to work normally.

### Provider Configuration

Embedding and synthesis providers are configurable via `.uf/dewey/config.yaml`:

```yaml
# Ollama (default — no config needed if defaults are fine)
embedding:
  provider: ollama
  model: granite-embedding:30m
  endpoint: http://localhost:11434
  max_chunk_chars: 12288  # optional: adjust for your model's context window

# Vertex AI — for cloud-powered embeddings or synthesis
embedding:
  provider: vertex
  model: text-embedding-005
  project: my-gcp-project
  region: us-central1

synthesis:
  provider: vertex
  model: claude-sonnet-4-6
  project: my-gcp-project
  region: us-east5
```

You can mix providers — e.g., Ollama for embeddings (fast, local) and Vertex AI for synthesis (high quality):

```yaml
embedding:
  provider: ollama
  model: granite-embedding:30m

synthesis:
  provider: vertex
  model: claude-sonnet-4-6
  project: my-gcp-project
  region: us-east5
```

Any OpenAI-compatible gateway (OpenRouter, ENMaas, LiteLLM, vLLM, or a self-hosted server) can serve either capability:

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

#### Vertex AI Provider

Vertex AI requires Google Cloud application-default credentials:

```bash
gcloud auth application-default login --scopes=https://www.googleapis.com/auth/cloud-platform
```

Set `project` and `region` in your config to match your GCP setup. Use a regional endpoint (e.g., `us-east5`, `us-central1`) or `global` to route requests to the nearest available region. Vertex AI embedding models (e.g., `text-embedding-005`) and Claude synthesis models (e.g., `claude-sonnet-4-6`) must be enabled in your GCP project.

**Note**: Switching embedding providers changes vector dimensions. Run `dewey reindex` after changing the embedding model.

**Rate limiting**: Vertex AI providers automatically retry on HTTP 429 (Too Many Requests) with exponential backoff and jitter — base delay 1 second, maximum 60 seconds, up to 5 attempts. The `Retry-After` header is respected when present. If retries are exhausted, the error is returned to the caller.

#### OpenAI-Compatible Gateway Provider

`provider: openai-compatible` speaks the OpenAI v1 wire protocol: synthesis via `POST {endpoint}/chat/completions`, embedding via `POST {endpoint}/embeddings`, and a `GET {endpoint}/models` availability probe. The configured `endpoint` is the API base URL (for example `https://gateway.example/v1`); a trailing slash or operation path is normalized away.

- `model` is sent verbatim — slashes and hyphens are not parsed or rewritten.
- `endpoint` has no Ollama fallback: `OLLAMA_HOST` and the localhost default do not apply, so a missing gateway endpoint is a configuration error naming `embedding.endpoint` or `synthesis.endpoint`.
- Endpoint precedence stays per code path: embedding is env-first (`DEWEY_EMBEDDING_ENDPOINT` over `config.yaml`); synthesis is config-first (`config.yaml` over `DEWEY_SYNTHESIS_ENDPOINT`).
- Availability is a cached (30-second) authenticated `GET /models` probe with a one-second timeout (embedding; the synthesis provider reports availability from configuration completeness alone). While it reports unavailable, semantic tools return the provider's reason and keyword/graph tools keep working.
- HTTP 429 responses retry with `Retry-After` or exponential backoff with jitter, matching Vertex (base 1 second, cap 60 seconds, one initial attempt plus at most five retries).

Gateway credentials resolve from environment variables — embedding keys are environment-only, while synthesis also honors `synthesis.api_key`:

| Variable | Applies to | Position |
|----------|------------|----------|
| `DEWEY_EMBEDDING_API_KEY` | embedding | first |
| `DEWEY_SYNTHESIS_API_KEY` | synthesis | first |
| `DEWEY_API_KEY` | embedding | second |
| `OPENROUTER_API_KEY` | embedding | third, only when the endpoint host is exactly `openrouter.ai` |

An empty variable counts as unset. For embedding, no key is required for an unauthenticated self-hosted gateway (the `Authorization` header is omitted) and an `api_key` field under `embedding:` in `config.yaml` is ignored — startup and `dewey doctor` warn about the ignored field. For synthesis, `DEWEY_SYNTHESIS_API_KEY` wins over `synthesis.api_key` (see the server-side synthesis section below). Startup and `dewey doctor` warn when a resolved embedding key would transit `http://` to a non-loopback host, and `dewey doctor` performs the same check for synthesis keys.

```bash
export DEWEY_SYNTHESIS_API_KEY="<your-key>"
dewey doctor   # reports gateway configuration and probe status without secrets
```

#### OpenAI-Compatible Synthesis (Server-Side)

Any provider that exposes the OpenAI chat-completions contract can perform synthesis. enmaas is the reference deployment:

```yaml
synthesis:
  provider: openai-compatible
  endpoint: https://api.enmaas.devshift.net/v1
  model: rits/zai-org/glm-5-3
  # api_key: prefer the environment variable instead of committing a key
  timeout: 300s       # optional, default 300s
  max_tokens: 16000   # optional, default 16000
```

Provide the credential with `DEWEY_SYNTHESIS_API_KEY` (recommended) or `synthesis.api_key`; the environment variable wins. The per-vault `.uf/dewey/config.yaml` is git-tracked (`dewey init` keeps it shareable for team config), so keep secrets in the environment variable or the global `~/.config/dewey/config.yaml`. The key is sent only in the `Authorization` header, is never logged, and is redacted from provider error bodies.

With a provider configured, `dewey serve` performs synthesis inside the MCP `compile` and `curate` tools, so agents do not need a CLI round-trip (the server holds the store lock while it runs). Requests retry HTTP 429 with the same bounded backoff as Vertex. A response that stops because it hit `max_tokens` (`finish_reason: "length"`) is reported as an error instead of silently returning truncated text — raise `max_tokens` or reduce the prompt size.

If no provider is configured or it cannot be constructed, `compile` and `curate` keep their prompt-mode behavior: they return the synthesis prompts for the calling agent to complete. Compiled articles can be persisted with `store_compiled`; curated content has no write-back MCP tool, so it requires a configured server-side synthesizer.

Configuring this provider sends learning and source content to the configured endpoint. Keep the local Ollama/Vertex setup if content must not leave your network. `DEWEY_SYNTHESIS_API_KEY` is read when the server builds the synthesizer, so restart `dewey serve` after rotating the key. `timeout` accepts a duration string (`300s`) or an integer number of seconds.

`compile` and `curate` hold the shared index lock while they synthesize, so `index`/`reindex` report "already in progress" during a long run. Prefer incremental compilation for large knowledge bases.

#### Global Config

Create `~/.config/dewey/config.yaml` (or `$XDG_CONFIG_HOME/dewey/config.yaml`) to set defaults for all vaults. Per-vault config overrides global.

#### Environment Variables

Environment variables override config file values for embedding. For synthesis, they serve as fallbacks when no config exists; the exception is `DEWEY_SYNTHESIS_API_KEY`, which always overrides `synthesis.api_key`.

| Variable | Default | Description |
|----------|---------|-------------|
| `DEWEY_EMBEDDING_MODEL` | `granite-embedding:30m` | Embedding model (overrides config) |
| `DEWEY_EMBEDDING_ENDPOINT` | `http://localhost:11434` | Embedding API endpoint (overrides config; required for `openai-compatible`) |
| `DEWEY_CHUNK_MAX_CHARS` | `12288` | Max chunk characters for embedding (overrides config) |
| `DEWEY_GENERATION_MODEL` | *(none)* | Synthesis model (fallback when no config) |
| `DEWEY_EMBEDDING_API_KEY` | *(none)* | Gateway embedding key (first in the credential chain) |
| `DEWEY_SYNTHESIS_API_KEY` | *(none)* | Gateway synthesis key (first in the credential chain; overrides `synthesis.api_key`) |
| `DEWEY_API_KEY` | *(none)* | Shared gateway key (second in the credential chain) |
| `OPENROUTER_API_KEY` | *(none)* | Gateway key used only for the exact `openrouter.ai` host |

## Knowledge Compilation

Dewey can synthesize stored learnings into compiled knowledge articles using an LLM. The `compile` tool reads all learnings, clusters them by topic tag, and produces current-state articles that cite their source learnings per claim. Contradictions are never resolved by recency: synthesis presents both positions with their citations, and every supersession is explicit — declared on a stored learning, or proposed for human confirmation. The compile result carries `proposed_supersessions` (successor/target pairs with rationale), `dropped_proposals` (proposals naming unknown learnings — reported, never applied), and per-article citation `coverage`; in `prompts_ready` mode the `store_compiled` results carry the same fields per stored page. Confirm a proposed pair with the `supersede_learning` tool; unconfirmed proposals surface again on the next lint.

When a synthesis provider is configured, the running MCP server performs compilation server-side: `compile` returns `status: "compiled"` and writes `.uf/dewey/compiled/{tag}.md`. If some topics fail it returns `status: "partial"` with a `failed` array and keeps the previous articles for those topics; if every topic fails it returns `status: "failed"` and changes nothing. Recompiling writes `tier: draft`; a `validated` article keeps its tier only when the regenerated article bytes are identical to the stored ones (content-bound preservation in the store), and changed content demotes to `draft` for re-review. The synthesizing model is recorded as `compiled_by`. When no provider is configured or available, `compile` returns synthesis prompts, the agent performs synthesis with any model, and `store_compiled` persists the result with provenance tracking. `store_compiled` refuses to overwrite an article whose tier is `validated`; re-open review or use a new tag instead. A full compile with no learnings and no synthesizer returns `prompts_ready` and leaves existing compiled artifacts untouched.

Learning markdown files are treated as untrusted backups during startup re-ingestion: a file that claims `tier: validated` is re-ingested as `draft` until a human promotes it again.

Use `/dewey-distill` to run lint → compile → curate → verify → promote through the running MCP server. The verify step runs `verify_candidates` over the run's candidate queue and presents each candidate's evidence locators and anchor-verification result (`anchors_ok`, `drifted`, `missing`, `no_anchors`, `skipped` with reasons) at the review gate, so the human approves a pre-verified queue; a skipped or failed pass is stated plainly and never presented as verified clean. It gates the synthesis scope, the supersession confirmations, and the promotion list on human approval — the review gate asks which proposed supersessions to confirm alongside the promotion list, applies confirmed pairs via `supersede_learning` before promoting, and aborts promotion when a confirmed apply errors or warns. It falls back to agent-side synthesis for compiled articles when server-side synthesis is unavailable. The command binds to the `curator` agent (`agent:` frontmatter) and runs `dewey gate auto-answer` at startup, refusing before any stage when the curator is configured for automatic question answering or the configuration cannot be verified; both mandatory gate questions carry the `HUMAN_GATE:` marker, which auto-answer plugins honoring the external contract leave for a human.

The `curator` agent is a least-privilege identity for knowledge work: `read` is enabled, `write`/`edit`/`webfetch` are denied, subagent spawning is denied, and the only shell command it may run is `dewey gate auto-answer`. Its Dewey MCP surface is limited to `lint`, `list_pages`, `compile`, `curate`, `store_compiled`, `verify_candidates`, `supersede_learning`, and `promote`; every other Dewey MCP tool is denied. Any supervisor guard relaxation for the curator belongs in the user's global supervisor configuration, never in a repository's committed config. Dewey ships the agent definition; `dewey init` scaffolds it into `.opencode/agents/` when `.opencode/` exists and never overwrites an existing file.

That boundary is enforced by the agent's frontmatter `tools:` map, not prompt-only policy: `"dewey_*": false` denies every Dewey MCP tool, the eight `true` entries that follow (`dewey_lint`, `dewey_list_pages`, `dewey_compile`, `dewey_curate`, `dewey_store_compiled`, `dewey_promote`, `dewey_verify_candidates`, `dewey_supersede_learning`) re-enable exactly the allowlisted tools, and `"replicator_*": false` denies tools from the other MCP servers configured in the repository-root `opencode.json`. The embedded `deweyAgentDefinitions["curator.md"]` copy in `agents.go` — the content `dewey init` scaffolds — ships byte-identical to the tracked file, and tests in `cli_test.go` assert the frontmatter shape and the parity between the frontmatter allow set and the prompt-text allowlist so the two lists cannot drift apart. The tool glob keys carry the MCP server key (`dewey`) from `opencode.json`; a repository configuring the Dewey MCP server under a different key must adjust the glob entries in the agent frontmatter.

### Trust Tiers

All content in Dewey's index has a trust tier:

| Tier | Source | Description |
|------|--------|-------------|
| `authored` | Disk, GitHub, web, code sources | Human-written content. Highest trust. |
| `curated` | `curate`, knowledge stores | Machine-extracted knowledge. LLM-curated with quality flags. |
| `validated` | `promote` | Agent content promoted by human review. |
| `draft` | `store_learning`, `compile` | Agent-generated. Unreviewed. |
| `untrusted` | Source config | Content from sources marked as lower trust. |

Filter search by tier: `semantic_search_filtered(query: "auth", tier: "authored")`
Filter curated content: `semantic_search_filtered(query: "auth", tier: "curated")`

### Knowledge Stores

Knowledge stores are named collections of curated knowledge extracted from indexed sources. Configure them in `.uf/dewey/knowledge-stores.yaml`:

```yaml
stores:
  - name: team-knowledge
    sources:
      - disk-meetings
      - github-org
    curation_interval: "10m"
```

Run `dewey curate` to extract knowledge, or let background curation handle it automatically during `dewey serve`. Curated files are written to `.uf/dewey/knowledge/{store-name}/` and automatically indexed with `tier: "curated"` for immediate searchability.

Each curated knowledge item includes:
- **Confidence scoring**: `high`, `medium`, `low`, or `flagged`
- **Quality flags**: `missing_rationale`, `implied_assumption`, `incongruent`, `unsupported_claim`
- **Source traceability**: Every fact traces back to its source document

Learnings stored via `store_learning` are also dual-written to `.uf/dewey/learnings/` as markdown files, ensuring they survive database deletion.

#### Temporal Validity

Learnings carry metadata about the code state they were true against, in their properties and markdown frontmatter (returned by every read tool):

| Field | Set by | Meaning |
|-------|--------|---------|
| `as_of` | automatic | `<repo>@<sha>` — the vault's git repository and `HEAD` commit when the learning was stored. Omitted when the vault is not a git repository. Uncommitted changes are not represented. |
| `evidence` | `evidence` input | Locators backing the learning: `<repo>:<path>`, `<repo>#<n>`, `learning/<identity>`, `session:<id>` (max 50, each max 512 characters). |
| `validity` | `validity` input | `current`, `stale-unverified`, `superseded`, or `historical`. Absent means unknown. |
| `supersedes` / `superseded_by` | `supersedes` input, or `supersede_learning` | Two-way link. Storing a learning with `supersedes: <identity>` marks the target `superseded_by` the new learning and `validity: superseded`, in the store and in its markdown file. Two existing learnings can be linked after the fact with the `supersede_learning` tool. The target is never deleted and its tier is unchanged; a learning can be superseded once. |

Concurrent supersessions of the same target are serialized at write time — the store claim is a compare-and-swap guarded on the target recording no successor, and the markdown backup is re-read and re-checked before it is rewritten — so exactly one successor is recorded. A call whose claim loses keeps its own learning and its `supersedes` claim, reports a non-empty `supersedes_warning` naming the winning successor, and does not overwrite the recorded successor.

Validity is separate from trust tiers: a tier says who vouched for a learning, validity says whether it still holds as the code evolves. All fields round-trip through re-ingestion.

### Content Sanitization

Dewey scans indexed content for security threats and structural anomalies through five defense layers:

1. **Injection pattern scanning** — Detects prompt injection attempts (instruction overrides, role reassignment, delimiter injection) using compiled regex patterns with severity calibration (critical/high/medium)
2. **Content hash drift detection** — Tracks content changes between index cycles via SHA-256 hash comparison
3. **Markdown structure validation** — Identifies invisible Unicode characters, embedded data URIs, invalid heading depth, and suspicious HTML tags (`<script>`, `<iframe>`, event handlers)
4. **Content size anomaly detection** — Statistical outlier detection using 3-sigma threshold across source documents (requires 5+ documents per source)
5. **Trust tier assignment** — Sources can be marked with explicit trust levels, with `untrusted` tier for lower-trust external content

#### Sanitization Configuration

Configure sanitization per-source in `.uf/dewey/sources.yaml`:

```yaml
sources:
  - name: external-wiki
    type: web
    config:
      urls:
        - https://wiki.example.com
    trust_tier: untrusted       # authored (default), curated, validated, draft, untrusted
    sanitize_mode: strict       # warn (default for web/github), strict, off (default for disk/code)
```

- **`sanitize_mode: warn`** — Scan content and merge findings into page properties. Content is still indexed. Default for `web` and `github` sources.
- **`sanitize_mode: strict`** — Reject documents with `critical` or `high` severity findings. Documents are skipped during indexing.
- **`sanitize_mode: off`** — Skip sanitization entirely. Default for `disk` and `code` sources (user-authored content).

#### Interpreting Findings

Use `dewey doctor` to see an aggregated summary of sanitization findings by source and severity:

```bash
dewey doctor --vault /path/to/vault
```

Use `dewey lint` to see per-page findings with severity and pattern version:

```bash
dewey lint --vault /path/to/vault
```

When patterns are updated in a new Dewey release, `dewey lint` flags stale findings and recommends `dewey reindex` to re-scan with updated patterns.

### Instant Startup

When `dewey serve` starts, the MCP server is ready within 1 second. Vault indexing runs in the background — tools serve from the persistent store (previous session's data) while indexing completes. The `health` tool reports indexing status.

## Architecture

```text
main.go              Entry point — backend routing, MCP server startup
cli.go               CLI subcommands: journal, add, search, init, index, reindex, status,
                     source, doctor, manifest, compile, lint, promote, curate, artifacts
gate.go              CLI gate subcommands: dewey gate auto-answer (deterministic startup gates)
agents.go            Shipped agent definitions scaffolded by dewey init
server.go            MCP server setup — 51 tool registrations
backend/backend.go   Backend interface + optional capability interfaces
client/logseq.go     Logseq HTTP API client with retry/backoff
vault/
  vault.go           Obsidian vault client — reads .md files into Backend interface
  markdown.go        Markdown → block tree parser (heading-based sectioning)
  frontmatter.go     YAML frontmatter parser
  index.go           Backlink index builder from [[wikilinks]]
  parse_export.go    Exported parsing and persistence functions
tools/
  navigate.go        Page, block, links, references, BFS traversal
  search.go          Full-text, property, DataScript/frontmatter, tag search
  analyze.go         Graph overview, connections, gaps, clusters
  write.go           Create, update, delete, move, link operations
  decision.go        Decision protocol: check, create, resolve, defer, analysis health
  journal.go         Date range and search within journals
  flashcard.go       SRS overview, due cards, card creation
  whiteboard.go      List and inspect whiteboards
  semantic.go        Semantic search, similar, filtered search, store_learning
  compile.go         Knowledge compilation, linting, promotion
  curate.go          Knowledge store curation pipeline handler
  helpers.go         Result formatting utilities
graph/
  builder.go         In-memory graph construction from any backend
  algorithms.go      Overview, connections, gaps, clusters, BFS
parser/content.go    Regex extraction of [[links]], ((refs)), #tags, properties
types/
  logseq.go          Shared types with custom JSON unmarshaling
  tools.go           Input types for all 51 tools
  semantic.go        Semantic search types
llm/                 LLM synthesis interface (Synthesizer, OllamaSynthesizer, VertexSynthesizer, OpenAICompatibleSynthesizer, NoopSynthesizer)
openaicompat/        Shared OpenAI-compatible transport (endpoint, auth, retry, errors, client, availability)
store/
  store.go           SQLite persistence (pages, blocks, links, sources)
  embeddings.go      Vector embedding storage and cosine similarity search
  migrate.go         Schema migration management
embed/
  embed.go           Embedder interface + Ollama implementation
  vertex.go          Vertex AI embedding implementation
  openai_compatible.go  OpenAI-compatible gateway embedding implementation
  provider.go        Provider factory + config
  chunker.go         Block-to-chunk preparation with heading context
source/
  source.go          Source interface definition
  config.go          Source configuration parsing (YAML)
  disk.go            Local disk source (file scanning)
  github.go          GitHub API source (issues, PRs, READMEs)
  web.go             Web crawl source (HTML-to-text, robots.txt)
  manager.go         Source orchestration (refresh, failures)
chunker/             Language-aware source code parsing (Go chunker, registry)
curate/              Knowledge store config parsing + curation pipeline
sanitize/            Content sanitization pipeline (injection scanning, structure validation,
                     drift detection, size anomaly detection)
artifacts/            Knowledge-artifact publishing: plan/sync sweep, screening, gitignore posture
```

## Attribution

Dewey is a hard fork of [graphthulhu](https://github.com/skridlevsky/graphthulhu), originally created by [Max Skridlevsky](https://github.com/skridlevsky). All graphthulhu functionality is preserved; Dewey extends it with additional capabilities for the Unbound Force ecosystem.

## Development

```bash
go build -o dewey .          # Build
go test ./...                # Test
go vet ./...                 # Vet
```

## License

[MIT](LICENSE)
