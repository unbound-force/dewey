# Contract: Configuration and Environment

**Feature**: `017-openai-compatible-providers` |
**Date**: 2026-11-02

This contract defines the user-facing configuration surface. It
complements `openai-compatible-wire.md`, which defines the HTTP wire
contract.

## Configuration File

`config.yaml` keeps its existing shape. No top-level section is added.

### Embedding

```yaml
embedding:
  provider: openai-compatible
  model: openrouter/amazon/titan-embed-text-v2
  endpoint: https://openrouter.ai/api/v1
  max_chunk_chars: 12288
```

### Synthesis

```yaml
synthesis:
  provider: openai-compatible
  model: enmaas/rits/zai-org/glm-5-3
  endpoint: https://gateway.example/v1
```

### Mixed Providers

Embedding and synthesis may use different providers, endpoints, and
credentials in one process.

```yaml
embedding:
  provider: openai-compatible
  model: openrouter/amazon/titan-embed-text-v2
  endpoint: https://openrouter.ai/api/v1

synthesis:
  provider: vertex
  model: claude-sonnet-4-6
  project: my-gcp-project
  region: us-east5
```

### Field Rules

| Field | `openai-compatible` behavior |
|-------|------------------------------|
| `provider` | Exact value `openai-compatible`; unknown values error and list `ollama`, `vertex`, `openai-compatible` |
| `model` | Required; sent verbatim |
| `endpoint` | Required; treated as API base URL; normalized |
| `project` | Ignored |
| `region` | Ignored |
| `max_chunk_chars` | Embedding only; existing behavior |

No API-key field is accepted. A key in `config.yaml` has no effect
and triggers a warning at startup and in `dewey doctor` (FR-024),
since keys are environment-only (FR-006).

## Environment Variables

### Endpoints

| Variable | Applies to | Precedence |
|----------|------------|------------|
| `DEWEY_EMBEDDING_ENDPOINT` | embedding | Above `config.yaml` (env-first) |
| `DEWEY_SYNTHESIS_ENDPOINT` | synthesis | Below `config.yaml` (config-first) |

For `openai-compatible`, `OLLAMA_HOST` and the
`http://localhost:11434` default are not consulted. A gateway endpoint
must come from the variable or the configuration file.

### Credentials

| Variable | Applies to | Precedence |
|----------|------------|------------|
| `DEWEY_EMBEDDING_API_KEY` | embedding | First |
| `DEWEY_SYNTHESIS_API_KEY` | synthesis | First |
| `DEWEY_API_KEY` | both | Second |
| `OPENROUTER_API_KEY` | both | Third, only on `openrouter.ai` |

An empty value counts as unset. No key is required for an
unauthenticated self-hosted gateway.

The layered credential chain, the endpoint-precedence asymmetry
(embedding env-first, synthesis config-first), and mixed-provider
setups form a non-obvious adoption workflow that is tutorial-worthy;
a tutorial tracking item is recorded as task T046 in `tasks.md`, to
be filed at implementation time.

## Error Message Contract

Factory failures name the exact configuration field:

```text
openai-compatible embedding provider requires endpoint (embedding.endpoint)
openai-compatible embedding provider requires model (embedding.model)
openai-compatible synthesis provider requires endpoint (synthesis.endpoint)
openai-compatible synthesis provider requires model (synthesis.model)
unknown embedding provider: "x" (supported: ollama, vertex, openai-compatible)
unknown synthesis provider: "x" (supported: ollama, vertex, openai-compatible)
```

Authentication failures name the expected environment variables and
never print the key:

```text
openai-compatible embedding endpoint requires authentication and no API key is available; set DEWEY_EMBEDDING_API_KEY or DEWEY_API_KEY
```

## Degradation Contract

When the gateway is unavailable:

- Keyword search, page navigation, graph tools, `health`, and
  `dewey status` keep working.
- Semantic tools return an error that includes the provider's
  unavailable reason when one is known.
- Compile and curate fall back to prompt mode when synthesis is
  unavailable.
- Non-semantic MCP tool contracts are unchanged.

## Backward Compatibility

- `provider` omitted still means `ollama`.
- Existing `ollama` and `vertex` sections, environment variables, and
  defaults behave exactly as before.
- No new module dependency is required.
- No database migration is required.
