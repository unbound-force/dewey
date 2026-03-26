## Context

The README covers the inherited graphthulhu features well (40 tools, Logseq/Obsidian setup, architecture) but is missing documentation for v0.2.0 additions: Homebrew cask install, persistence layer, content source configuration, and Ollama semantic search setup. These gaps prevent developers from discovering and using Dewey's key differentiators.

Per the proposal, the constitution alignment is PASS/N/A — this is a documentation-only change with no production code impact.

## Goals / Non-Goals

### Goals
- Add Homebrew cask install as the primary macOS method, ordered before `go install`
- Document the persistence layer (`.dewey/` directory, `graph.db`, incremental indexing)
- Add a complete `sources.yaml` example covering all 3 source types
- Add Ollama setup instructions (install, model pull, verification)
- Ensure all documented commands match actual `dewey --help` output for v0.2.0

### Non-Goals
- Rewriting existing README sections (Logseq setup, Obsidian setup, tools tables) — these are already accurate
- Adding a CHANGELOG — separate concern, not part of this change
- Adding `doc.go` package-level documentation — separate concern
- Documenting features planned for future versions

## Decisions

### 1. Section ordering in Install

Add Homebrew cask first, then `go install`, then build-from-source. Homebrew is the most common macOS developer tool manager and should be the primary path. The `go install` path remains for Linux and for developers who prefer it.

### 2. Persistence section placement

Add a new "Persistence" section between "Configuration" and "CLI Commands". This is where users would naturally look after configuring their backend — "what happens when I restart Dewey?"

### 3. Content sources section

Add a "Content Sources" section after "CLI Commands" with a complete `sources.yaml` example. This ties the `dewey source add`, `dewey index`, and `dewey status` commands together with the YAML configuration they produce.

### 4. Semantic search section

Add a "Semantic Search Setup" section after "Content Sources" covering Ollama installation, model pull, and verification. This ties the semantic search tools in the tools table to the setup required to use them. Document graceful degradation (keyword tools work without Ollama).

### 5. Accuracy verification

Cross-reference all documented commands against `dewey --help` and subcommand help output before marking tasks complete. This aligns with Observable Quality — documentation must be verifiable.

## Risks / Trade-offs

- **README length**: Adding 4 new sections increases README size. Mitigated by keeping each section concise (5-15 lines) and using code blocks for examples.
- **Content staleness**: Future Dewey versions may change CLI flags or config format. Mitigated by documenting only v0.2.0 features and noting the version in the section headers where appropriate.
- **Homebrew cask availability**: The cask may not be immediately available after the formula→cask transition. Mitigated by documenting `go install` as the fallback.
