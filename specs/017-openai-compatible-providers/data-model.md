# Data Model: OpenAI-Compatible API Providers

**Feature**: `017-openai-compatible-providers` |
**Date**: 2026-11-02

## Scope

No persistent schema change is introduced. The SQLite tables,
`pages`, `blocks`, `links`, and `embeddings`, are unchanged. The
entities below are configuration values, transient credentials, and
in-memory request and response types. They are documented because the
feature's contract depends on their validation and ordering rules.

## Entity: Provider Configuration

Source: `config.yaml`, section `embedding` or `synthesis`.

| Field | Type | Embedding | Synthesis | Notes |
|-------|------|-----------|-----------|-------|
| `provider` | string | optional | optional | `openai-compatible` selects the new provider. Empty means `ollama`. |
| `model` | string | required | required | Opaque identifier passed verbatim, including slashes and hyphens. |
| `endpoint` | string | required | required | API base URL. No Ollama fallback for this provider. |
| `project` | string | vertex only | vertex only | Unused by `openai-compatible`. |
| `region` | string | vertex only | vertex only | Unused by `openai-compatible`. |
| `max_chunk_chars` | integer | optional | not read | Embedding chunk limit; existing behavior. |

### Validation rules

- `provider` must be one of `ollama`, `vertex`, or
  `openai-compatible`; any other value produces a factory error that
  lists all three supported values (FR-016, US4-4).
- `openai-compatible` without `endpoint` produces an error naming
  `embedding.endpoint` or `synthesis.endpoint` (FR-009).
- `openai-compatible` without `model` produces an error naming
  `embedding.model` or `synthesis.model` (US1-7, US2-6).
- An unset `provider` remains `ollama` (FR-017).
- No credential field is accepted or written in this entity (FR-006).

### Source precedence

- Embedding, `openai-compatible`:
  `DEWEY_EMBEDDING_ENDPOINT`, then per-vault
  `embedding.endpoint`, then global `embedding.endpoint`. No
  `OLLAMA_HOST`, no localhost default (FR-022).
- Synthesis, `openai-compatible`:
  per-vault `synthesis.endpoint`, then global `synthesis.endpoint`,
  then `DEWEY_SYNTHESIS_ENDPOINT`. No `OLLAMA_HOST`, no localhost
  default (FR-022).
- `model` follows the existing per-vault then global then environment
  rules for each capability.

## Entity: Credential Reference

A transient environment reference, never persisted.

| Variable | Scope | Position in chain |
|----------|-------|-------------------|
| `DEWEY_EMBEDDING_API_KEY` | embedding | first |
| `DEWEY_SYNTHESIS_API_KEY` | synthesis | first |
| `DEWEY_API_KEY` | both | second |
| `OPENROUTER_API_KEY` | both | third, only when the endpoint host is `openrouter.ai` |

### Invariants

- Resolution order is fixed and capability-scoped (FR-007).
- An empty variable is treated as unset.
- A key is never written to `config.yaml`, the knowledge store, logs,
  error messages, or generated artifacts (FR-006, FR-019).
- When no key resolves, no `Authorization` header is sent (FR-008
  clarification).
- Any occurrence of the resolved key in a response excerpt is replaced
  with `[REDACTED]` before the excerpt reaches an error.

## Entity: Embedding Vector

Produced by one embedding request and stored per input text.

| Field | Type | Notes |
|-------|------|-------|
| `index` | integer | Position supplied by the gateway; used only to restore order. |
| `embedding` | array of numbers | Converted to `float32` before storage. |

### Invariants

- Exactly one vector per input, in the original input order (FR-023).
- A count mismatch, duplicate index, or out-of-range index is an error.
- Every persisted vector is non-empty; a zero-length `embedding` array is
  rejected as an error naming the offending index (a zero-norm vector yields
  NaN cosine similarity).
- An empty input list returns an empty result without a network call
  (US2-5).
- Non-ASCII text round-trips unchanged as UTF-8 JSON.

## Entity: Completion

The assistant text returned by one synthesis request.

| Field | Type | Notes |
|-------|------|-------|
| `choices[0].message.content` | string | First choice only. |

### Invariants

- Missing, null, or empty content is a descriptive error, never an
  empty article (US1-6).
- Requests are non-streaming and do not use tools, function calling,
  or structured output (FR-021).

## Internal Type: openaicompat.Client

Not persisted. Owned by one provider instance.

| Field | Type | Purpose |
|-------|------|---------|
| `baseURL` | string | Normalized base URL, no trailing slash. |
| `apiKey` | string | Resolved credential, never logged or formatted. |
| `httpClient` | `*http.Client` | Embedding uses 30s, synthesis 300s (FR-025). |
| `mu` | `sync.RWMutex` | Guards the availability cache. |
| `available` | bool | Last probe outcome. |
| `lastCheck` | `time.Time` | When the probe last ran. |
| `checkExpiry` | `time.Duration` | 30 seconds. |
| `lastReason` | string | Credential-free unavailable reason. |

### Concurrency invariants

- `Available()` may run concurrently with `Embed`, `EmbedBatch`, and
  `Synthesize` (edge case: concurrent access).
- Double-checked locking prevents duplicate probes when several
  callers miss the cache at once.
- No request-scoped mutable state is shared between calls.

## State Transitions: Availability Cache

| Current | Trigger | Next | Duration |
|---------|---------|------|----------|
| unchecked | first `Available()` call | probing | one request |
| probing | 2xx from `/models` | available | 30 seconds |
| probing | connection error, 401, 403, other status | unavailable plus reason | 30 seconds |
| available | cache expiry | probing | one request |
| unavailable | cache expiry | probing | one request |

A cached result is returned without a network call. Cancellation is
not applicable because `Available()` has no context parameter; the
probe uses an internal one-second timeout.

## Relationships

```text
config.yaml (embedding | synthesis)
        │
        ▼
ReadEmbeddingConfig / ReadSynthesisConfig  →  ProviderConfig
        │
        ▼
NewEmbedderFromConfig / NewSynthesizerFromConfig
        │  (resolves env credential, validates fields)
        ▼
OpenAICompatibleEmbedder / OpenAICompatibleSynthesizer
        │
        ▼
openaicompat.Client  →  gateway HTTP
        │
        ├─ vectors → store.Store (embedding)
        └─ text    → compiled article or curation output
```
