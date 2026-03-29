## Context

Currently, both `dewey serve` (`main.go:215-220`) and `dewey index` (`cli.go:680-685`) check `embedder.Available()` and log a WARN if the model is unavailable. Execution continues without embeddings. This is misleading — users think Dewey is fully operational but semantic search silently returns nothing.

The Ollama embedder (`embed/embed.go`) checks availability by making a POST to `{endpoint}/api/embeddings` with the configured model. It fails if either Ollama isn't running or the model isn't pulled.

## Goals / Non-Goals

### Goals
- Make embedding unavailability a hard error with a clear fix instruction
- Provide `--no-embeddings` flag for users who intentionally don't want semantic search
- Add `dewey doctor` command for comprehensive prerequisite checking

### Non-Goals
- Auto-pulling the model (would violate the "no data leaves machine without consent" principle)
- Changing the semantic search tools' behavior (they already return clear errors when embedder is nil)
- Modifying the embed package itself

## Decisions

**D1: Hard error with actionable message**

When `embedder.Available()` returns false, return an error like:
```
embedding model "granite-embedding:30m" not available at http://localhost:11434

To fix:
  ollama pull granite-embedding:30m

To skip embeddings:
  dewey serve --no-embeddings
```

**D2: `--no-embeddings` flag on serve, index, and root command**

A boolean flag that skips embedder creation entirely. When set:
- No embedder is created
- No availability check runs
- Semantic search tools return "embeddings disabled" instead of "model not available"
- `dewey status` shows "embeddings: disabled (--no-embeddings)"

The flag goes on the root command (inherited by serve) and on the index command separately.

**D3: `dewey doctor` checks**

The doctor command runs these checks in order:

| Check             | Pass                               | Fail                          | Fix                                                  |
|-------------------|-------------------------------------|-------------------------------|------------------------------------------------------|
| `.dewey/` exists  | `.dewey/ found at /path`            | `.dewey/ not found`           | `dewey init`                                         |
| `graph.db` exists | `graph.db: 1,234 pages`             | `graph.db not found or empty` | `dewey index`                                        |
| Ollama reachable  | `Ollama running at localhost:11434` | `Ollama not reachable`        | `brew install --cask ollama-app && open -a Ollama`   |
| Embedding model   | `granite-embedding:30m available`   | `model not found`             | `ollama pull granite-embedding:30m`                  |

Output format: one line per check with pass/fail indicator and fix command.

## Risks / Trade-offs

**Breaking change**: Users who previously relied on silent degradation will now get an error. The `--no-embeddings` flag is the migration path — if they add it, behavior is identical to before. This is intentional: silent degradation was causing confusion.

**Doctor output format**: Plain text for v1. Could add `--json` later for machine parsing.
