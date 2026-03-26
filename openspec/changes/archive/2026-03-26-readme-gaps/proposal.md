## Why

The Dewey README is missing documentation for four major features that shipped in v0.2.0:

1. **No Homebrew cask install** — The Install section only shows `go install` and build-from-source. The primary macOS distribution method (`brew install --cask unbound-force/tap/dewey`) is undocumented.
2. **No persistence documentation** — The README mentions persistence in the intro but never explains the `.dewey/graph.db` database, incremental indexing, or what `dewey init` creates.
3. **No content sources guide** — The CLI section documents `dewey source add` and `dewey index` but doesn't show a complete `sources.yaml` example with all three source types (disk, GitHub, web crawl).
4. **No Ollama/semantic search setup** — The semantic search tools are listed in the tools table, but there's no setup guide for Ollama or the embedding model.

A developer reading the README today would not know how to install Dewey via Homebrew, set up semantic search, or configure cross-repository content sources.

## What Changes

Update `README.md` to document all v0.2.0 features with accurate commands and configuration examples.

## Capabilities

### New Capabilities
- `homebrew-install-docs`: Homebrew cask install command and instructions in the Install section
- `persistence-docs`: Explanation of `.dewey/` directory, `graph.db`, and incremental indexing
- `sources-config-docs`: Complete `sources.yaml` example with disk, GitHub, and web crawl sources
- `semantic-search-setup-docs`: Ollama installation, model pull, and verification steps

### Modified Capabilities
- `install-section`: Add Homebrew cask as the primary macOS install method above `go install`
- `configuration-section`: Expand with persistence and sources information
- `cli-commands-section`: Minor refinements to match actual v0.2.0 help output

### Removed Capabilities
- None

## Impact

Single file affected: `README.md`. No production code changes. No test changes. No CI changes.

## Constitution Alignment

Assessed against the Unbound Force org constitution.

### I. Autonomous Collaboration

**Assessment**: N/A

README documentation does not affect artifact-based communication or MCP tool contracts. No runtime behavior changes.

### II. Composability First

**Assessment**: PASS

The README updates reinforce composability by documenting Dewey as independently installable (Homebrew cask, go install, build from source) without requiring any other Unbound Force tool. The Ollama setup section documents it as optional with graceful degradation.

### III. Observable Quality

**Assessment**: PASS

The README will document `dewey status` for health reporting and `dewey index` for auditable source management. Users can verify their setup produces correct output.

### IV. Testability

**Assessment**: N/A

Documentation-only change. No production code or test infrastructure affected.
