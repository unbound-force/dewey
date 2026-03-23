# CLI Command Contracts: Dewey

**Branch**: `001-core-implementation` | **Date**: 2026-03-22

Four new CLI subcommands. Existing subcommands (`serve`, `journal`, `add`, `search`, `version`) remain unchanged.

## dewey init

**Purpose**: Initialize a Dewey configuration in the current directory.

**Usage**:
```
dewey init [--vault PATH]
```

**Flags**:
- `--vault PATH`: Path to the vault root. Default: current directory.

**Behavior**:
1. Creates `.dewey/` directory if it does not exist.
2. Creates `.dewey/config.yaml` with default configuration (disk source only, default embedding model).
3. Creates `.dewey/sources.yaml` with a single disk source entry.
4. Adds `.dewey/` to `.gitignore` if a `.gitignore` exists and `.dewey/` is not already listed.
5. Prints confirmation message to stderr.

**Output** (stderr):
```
dewey: initialized .dewey/ in /path/to/repo
dewey: default config: .dewey/config.yaml
dewey: run 'dewey index' to build the initial index
```

**Error cases**:
- `.dewey/` already exists: Print "dewey: already initialized" and exit 0 (idempotent).

---

## dewey index

**Purpose**: Build or update the knowledge graph and embedding indexes.

**Usage**:
```
dewey index [--source NAME] [--force]
```

**Flags**:
- `--source NAME`: Index only the specified source. Default: all sources.
- `--force`: Force full re-index, ignoring cached content and refresh intervals.

**Behavior**:
1. Loads configuration from `.dewey/config.yaml` and `.dewey/sources.yaml`.
2. For each configured source (or the specified source):
   a. Check refresh interval. Skip if within interval and `--force` not set.
   b. Fetch content (disk: scan files; github: API calls; web: HTTP crawl).
   c. Index content into the knowledge graph (pages, blocks, links).
   d. Generate embeddings if the embedding model is available.
   e. Update `last_fetched_at` timestamp.
3. Report summary to stderr.

**Output** (stderr):
```
dewey: indexing disk-local... 188 pages (3 changed)
dewey: indexing github-gaze... 47 pages (12 new, 5 updated)
dewey: generating embeddings... 1523/1600 blocks
dewey: index complete. 235 pages, 1523 embeddings.
```

**Error cases**:
- `.dewey/` not initialized: Print "dewey: not initialized. Run 'dewey init' first." Exit 1.
- Source fetch failure: Print warning, continue with other sources. Exit 0.
- Embedding model unavailable: Print warning, skip embedding generation. Index is still built. Exit 0.

---

## dewey status

**Purpose**: Report index health and source status.

**Usage**:
```
dewey status [--json]
```

**Flags**:
- `--json`: Output as JSON instead of human-readable text.

**Behavior**:
1. Loads index from `.dewey/`.
2. Reports: page count, block count, embedding count, embedding model, embedding availability, source list with status and freshness.

**Human-readable output** (stdout):
```
Dewey Index Status
  Path:       .dewey/
  Pages:      235
  Blocks:     1600
  Embeddings: 1523 (95% coverage)

Embedding Model
  Model:      granite-embedding:30m
  Available:  yes
  Dimension:  384

Sources
  disk-local     active   188 pages  last indexed 2m ago
  github-gaze    active    47 pages  last fetched 4h ago
  web-go-stdlib  error      0 pages  failed: connection timeout
```

**JSON output**:
Same structure as the `dewey` field in the updated `health` MCP tool response.

**Error cases**:
- `.dewey/` not initialized: Print "dewey: not initialized. Run 'dewey init' first." Exit 1.

---

## dewey source add

**Purpose**: Add a content source to the configuration.

**Usage**:
```
dewey source add github --org ORG --repos REPO1,REPO2 [--refresh INTERVAL]
dewey source add web --url URL [--name NAME] [--depth N] [--refresh INTERVAL]
```

**Flags** (github):
- `--org ORG`: GitHub organization name. Required.
- `--repos REPO1,REPO2`: Comma-separated list of repository names. Required.
- `--content issues,pulls,readme`: Content types to fetch. Default: `issues,pulls,readme`.
- `--refresh INTERVAL`: Refresh interval. Default: `daily`.

**Flags** (web):
- `--url URL`: Documentation URL to crawl. Required.
- `--name NAME`: Human-readable source name. Default: derived from URL hostname.
- `--depth N`: Crawl depth. Default: 1.
- `--refresh INTERVAL`: Refresh interval. Default: `weekly`.

**Behavior**:
1. Validates arguments.
2. Appends source entry to `.dewey/sources.yaml`.
3. Prints confirmation to stderr.

**Output** (stderr):
```
dewey: added source github-gaze (github, repos: gaze, refresh: daily)
dewey: run 'dewey index --source github-gaze' to fetch content
```

**Error cases**:
- `.dewey/` not initialized: Print "dewey: not initialized. Run 'dewey init' first." Exit 1.
- Source with same name already exists: Print "dewey: source github-gaze already exists" and exit 1.
- Invalid arguments: Print usage and exit 1.
