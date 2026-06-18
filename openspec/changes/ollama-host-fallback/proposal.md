## Why

Dewey does not read the ecosystem-standard `OLLAMA_HOST` environment variable
and hard-exits when an embedding model is missing (GitHub issue #61). This
causes two user-facing problems:

1. Users who set `OLLAMA_HOST` (which `uf doctor` and other tools rely on) find
   that Dewey connects to `localhost:11434` instead of their configured endpoint.
2. When Ollama is reachable but the embedding model has not been pulled, `dewey
   serve` exits with an error instead of degrading gracefully to keyword-only
   mode — inconsistent with its existing behavior when Ollama is completely
   unavailable.

Additionally, `dewey doctor` reads `DEWEY_EMBEDDING_ENDPOINT` directly instead
of calling `ReadEmbeddingConfig()`, so it can report a different endpoint than
`serve`/`index` actually use. The default endpoint `http://localhost:11434` is
hardcoded in three separate locations, creating a maintenance risk.

## What Changes

### Env var fallback chain

All Ollama endpoint resolution paths adopt a three-tier fallback:
1. `DEWEY_EMBEDDING_ENDPOINT` (app-specific override, preserves backward compat)
2. `OLLAMA_HOST` (ecosystem-standard fallback)
3. `http://localhost:11434` (hardcoded default)

### Graceful degradation on missing model

When Ollama is reachable but the requested embedding model is not available,
Dewey logs a warning and continues without embeddings (keyword-only mode) instead
of returning a fatal error. This matches existing behavior when Ollama is
completely unavailable.

### Doctor endpoint consistency

`dewey doctor` calls `ReadEmbeddingConfig()` instead of reading
`DEWEY_EMBEDDING_ENDPOINT` directly, ensuring it reports the same endpoint that
`serve` and `index` use.

### Default endpoint constant

Extract the hardcoded `http://localhost:11434` default into a single constant
shared across `embed/config.go`, `embed/provider.go`, and `cli.go`.

## Capabilities

### New Capabilities
- `OLLAMA_HOST fallback`: Dewey reads the ecosystem-standard `OLLAMA_HOST` env var when `DEWEY_EMBEDDING_ENDPOINT` is not set.

### Modified Capabilities
- `Embedding model check`: Missing model triggers a warning and keyword-only mode instead of a fatal exit.
- `dewey doctor`: Uses centralized config resolution instead of independent env var read.

### Removed Capabilities
- None.

## Impact

- **Files**: `embed/config.go`, `embed/provider.go`, `llm/config.go`, `main.go`, `cli.go`
- **Behavior**: Users with `OLLAMA_HOST` set will now have Dewey connect to their configured endpoint without needing `DEWEY_EMBEDDING_ENDPOINT`. Users without the embedding model pulled will see Dewey start in keyword-only mode instead of exiting.
- **Backward compatibility**: Fully preserved. `DEWEY_EMBEDDING_ENDPOINT` takes precedence over `OLLAMA_HOST`, and `config.yaml` `embedding.endpoint` still overrides both. Existing users who do not set `OLLAMA_HOST` see no change.
- **Out of scope**: The synthesis provider's reuse of `DEWEY_EMBEDDING_ENDPOINT` (finding 4 from triage) is tracked as a separate issue.

## Constitution Alignment

Assessed against the Unbound Force org constitution.

### I. Composability First

**Assessment**: PASS

Dewey remains independently installable and usable without any other Unbound
Force tool. The `OLLAMA_HOST` fallback improves interop with the broader Ollama
ecosystem but introduces no mandatory dependency. Graceful degradation on
missing model strengthens standalone usability — Dewey starts successfully in
more environments without manual configuration.

### II. Autonomous Collaboration

**Assessment**: PASS

This change does not alter MCP tool contracts or artifact-based communication.
All 50 MCP tools continue to produce identical structured JSON responses.
The embedding configuration is internal to Dewey's startup sequence.

### III. Observable Quality

**Assessment**: PASS

The change improves observability: Dewey will log which endpoint it resolved and
whether it degraded to keyword-only mode. `dewey doctor` will report the actual
endpoint used by `serve`/`index` instead of potentially diverging.

### IV. Testability

**Assessment**: PASS

All changes are testable in isolation. `ReadEmbeddingConfig()` already has unit
tests for env var override behavior — tests will be extended to cover `OLLAMA_HOST`
fallback. The graceful degradation path is testable via the existing
`initObsidianBackend` test infrastructure with mock embedders. No external
services required.
