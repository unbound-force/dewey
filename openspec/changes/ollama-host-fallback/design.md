## Context

Dewey resolves the Ollama endpoint through `embed.ReadEmbeddingConfig()` which
reads `DEWEY_EMBEDDING_ENDPOINT` from env, then `config.yaml`, then defaults to
`http://localhost:11434`. The ecosystem-standard `OLLAMA_HOST` env var is not
checked anywhere. When the embedding model is missing but Ollama is reachable,
Dewey hard-exits instead of degrading gracefully. Additionally, `dewey doctor`
bypasses `ReadEmbeddingConfig()` entirely and reads env vars directly, potentially
reporting a different endpoint than `serve`/`index` use.

See proposal.md for the full motivation and constitution alignment assessment.

## Goals / Non-Goals

### Goals
- Add `OLLAMA_HOST` as a fallback env var in the endpoint resolution chain
- Degrade gracefully when the embedding model is missing (warn + keyword-only mode)
- Make `dewey doctor` use `ReadEmbeddingConfig()` for consistent endpoint reporting
- Extract the default Ollama endpoint into a shared constant

### Non-Goals
- Introducing a `DEWEY_SYNTHESIS_ENDPOINT` env var (separate issue, synthesis endpoint conflation)
- Changing `config.yaml` schema or fields
- Modifying Vertex AI provider paths (no `OLLAMA_HOST` relevance)
- Adding `OLLAMA_HOST` support to `llm/config.go` synthesis paths (tracked separately)

## Decisions

### D1: Fallback chain order

Full precedence (highest to lowest):
1. `DEWEY_EMBEDDING_ENDPOINT` env var (app-specific override)
2. `config.yaml` `embedding.endpoint` (per-vault, then global)
3. `OLLAMA_HOST` env var (ecosystem-standard fallback)
4. `DefaultOllamaEndpoint` constant

**Rationale**: `DEWEY_EMBEDDING_ENDPOINT` is app-specific and must win for users
who explicitly set it. Config file values sit next per existing `ReadEmbeddingConfig()`
behavior. `OLLAMA_HOST` is the ecosystem standard that Ollama itself, `uf doctor`,
and other tools read — it serves as a fallback when no Dewey-specific config sets
the endpoint. This preserves full backward compatibility — no existing user
behavior changes.

**Note**: `OLLAMA_HOST` is only inserted into the env var fallback, not into the
config file resolution. `ReadEmbeddingConfig()` reads config.yaml first, then
overrides with `DEWEY_EMBEDDING_ENDPOINT`. The `OLLAMA_HOST` fallback only
applies when neither `DEWEY_EMBEDDING_ENDPOINT` nor `config.yaml` provides an
endpoint — i.e., it is used by `resolveOllamaEndpoint()` which is called as the
default when no higher-priority source sets the endpoint.

### D2: Centralize endpoint resolution into a helper

Introduce a `ResolveOllamaEndpoint()` exported function in `embed/config.go`
that encapsulates the env var fallback: `DEWEY_EMBEDDING_ENDPOINT` →
`OLLAMA_HOST` → `DefaultOllamaEndpoint`. If the resolved value lacks a URL
scheme (no `://`), prepend `http://`. Empty strings are treated as unset.

**Call sites**:
- `ReadEmbeddingConfig()` line 83: replace the `DEWEY_EMBEDDING_ENDPOINT`-only
  check with `ResolveOllamaEndpoint()`. This is only called when `config.yaml`
  provides no endpoint (or has an empty endpoint field).
- `embedConfigFromEnv()` line 101-103: replace the hardcoded fallback with
  `ResolveOllamaEndpoint()`.
- `NewEmbedderFromConfig()` in `provider.go` line 33-34: use
  `DefaultOllamaEndpoint` constant (the full resolution has already run by the
  time this function is called; this is just a defensive fallback).

The helper does NOT read `config.yaml` — that is `ReadEmbeddingConfig()`'s
responsibility. `ResolveOllamaEndpoint()` only handles the env var + default
portion of the chain.

**Rationale**: Single source of truth (Coding Standards principle I). The current
three hardcoded `http://localhost:11434` strings in `embed/config.go:103`,
`embed/provider.go:34`, and `cli.go:1353` are a divergence risk.

### D3: Warn-and-continue on missing model

When `embedder.Available()` returns false, log a warning with the model name,
endpoint, and `ollama pull` instructions, then continue without embeddings. This
matches the existing `OllamaUnavailable` path at `main.go:633-638`.

**Rationale**: Composability First — Dewey should start successfully in as many
environments as possible. The user already sees the `--no-embeddings` hint in the
current error message, confirming that keyword-only mode is a valid operating
state. Making it automatic removes a startup barrier.

The warning MUST include the endpoint being used, so users can diagnose
misconfigured endpoints (e.g., Dewey hitting `localhost:11434` when Ollama is on
a different host).

### D4: Doctor uses ReadEmbeddingConfig

Replace the direct `os.Getenv("DEWEY_EMBEDDING_ENDPOINT")` read in `cli.go`'s
doctor command with a call to `embed.ReadEmbeddingConfig(deweyDir)`. This ensures
doctor reports the same endpoint and model that `serve`/`index` resolve.

**Rationale**: Observable Quality — doctor's diagnostic output must reflect the
actual runtime configuration, not a simplified approximation.

### D5: Default endpoint constant

Define `DefaultOllamaEndpoint = "http://localhost:11434"` as an exported constant
in the `embed` package. All references to this value in `embed/`, `llm/`, and
`cli.go` use this constant.

**Rationale**: Single source of truth. The constant lives in `embed` because
that's the primary consumer and the package that owns endpoint resolution.

## Coverage Strategy

All new functions (`ResolveOllamaEndpoint`) and modified branches (missing-model
paths in `initObsidianBackend` and `createIndexEmbedder`) will have dedicated
unit tests. Tests use `t.Setenv()` for env var isolation and in-memory fixtures
— no external services required. Gaze CI ratchets (`--max-crapload=48`,
`--max-gaze-crapload=18`, `--min-contract-coverage=70`) enforce no coverage
regression.

## Risks / Trade-offs

### Risk: OLLAMA_HOST format differences

Ollama's own documentation shows `OLLAMA_HOST` can be set as `host:port` (no
scheme) or as a full URL. Dewey currently expects a full URL with scheme
(e.g., `http://localhost:11434`). If a user sets `OLLAMA_HOST=0.0.0.0:11434`,
the HTTP client will fail.

**Mitigation**: The `resolveOllamaEndpoint()` helper will normalize the value —
if no scheme is present, prepend `http://`. This matches how Ollama's own client
libraries handle the variable.

### Risk: Silent degradation hides real problems

Automatically continuing without embeddings when the model is missing could mask
configuration errors (e.g., pointing at the wrong Ollama instance).

**Mitigation**: The warning log includes the endpoint URL, model name, and
`ollama pull` instructions. Users who check logs will see the issue. Users who
don't check logs were already blocked by the hard exit, so this is strictly
better — they get a working (if reduced) Dewey instead of nothing.

### Trade-off: llm/config.go gets constant but not OLLAMA_HOST

This change updates `llm/config.go` to use the `DefaultOllamaEndpoint` constant
but does not add `OLLAMA_HOST` fallback to the synthesis endpoint paths. That's
intentional — the synthesis endpoint conflation is a separate concern tracked in
a separate issue. Mixing it here would expand scope.
