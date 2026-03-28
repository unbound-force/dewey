## Why

When Ollama is not running or the embedding model is not pulled, `dewey serve` and `dewey index` silently degrade — they log a WARN and skip embedding generation. This means users get zero semantic search results with no clear guidance on how to fix it. The warning is easy to miss in log output, especially when Dewey is started as an MCP server by an AI agent.

Two improvements:

1. **Hard error by default**: If the embedding model is unavailable, `dewey serve` and `dewey index` should fail with a clear error message telling the user exactly what to run (`ollama pull granite-embedding:30m`). A `--no-embeddings` flag allows users who intentionally don't want semantic search to opt out.

2. **`dewey doctor` command**: A prerequisite checker that reports the health of all Dewey dependencies (Ollama running, model pulled, `.dewey/` initialized, graph.db exists) with actionable fix instructions for each.

## What Changes

- Change Ollama unavailability from a WARN to a hard error in both `dewey serve` and `dewey index`
- Add `--no-embeddings` flag to both commands to explicitly opt out of embedding generation
- Add `dewey doctor` subcommand that checks prerequisites and prints actionable diagnostics

## Capabilities

### New Capabilities
- `cli-doctor`: New `dewey doctor` command that checks Ollama availability, embedding model status, `.dewey/` initialization, and graph.db health. Prints pass/fail for each check with fix instructions.
- `no-embeddings-flag`: `--no-embeddings` flag on `dewey serve` and `dewey index` to explicitly skip embedding generation without error.

### Modified Capabilities
- `cli-serve`: `dewey serve` now fails with an actionable error when the embedding model is unavailable, unless `--no-embeddings` is passed.
- `cli-index`: `dewey index` now fails with an actionable error when the embedding model is unavailable, unless `--no-embeddings` is passed.

### Removed Capabilities
- None

## Impact

- **`main.go`**: Change `initObsidianBackend()` to error on embedding unavailability. Add `--no-embeddings` flag to root and serve commands.
- **`cli.go`**: Change `createIndexEmbedder()` to error on unavailability. Add `--no-embeddings` flag to index command. Add `newDoctorCmd()`.
- **User impact**: Users who have Ollama installed but haven't pulled the model will see a clear error with the exact command to fix it. Users who don't want embeddings can pass `--no-embeddings`.

## Constitution Alignment

Assessed against the Unbound Force org constitution.

### I. Autonomous Collaboration

**Assessment**: PASS

The `dewey doctor` command produces structured, self-describing output that agents can parse. The hard error on embedding unavailability gives agents a clear signal instead of a silent degradation they can't detect.

### II. Composability First

**Assessment**: PASS

Dewey remains independently usable. The `--no-embeddings` flag preserves the ability to run without Ollama. The doctor command checks for optional dependencies without requiring them.

### III. Observable Quality

**Assessment**: PASS

The hard error and doctor command both improve observability — users and agents know exactly what's working and what's not, with actionable remediation steps.

### IV. Testability

**Assessment**: PASS

The doctor command checks can be unit-tested by mocking the Ollama HTTP endpoint and filesystem state. The `--no-embeddings` flag is a simple boolean that bypasses the embedder creation path.
