## Why

The Ollama embed client in `embed/embed.go` sends a bare JSON string for the `input` field when embedding a single text (`"input": "text"`). The Ollama `/api/embed` endpoint accepts both string and array forms, but some Ollama versions and proxies (e.g., `uf ollama-proxy`) strictly require the array form (`"input": ["text"]`). This causes `dewey reindex` and semantic search queries to fail with HTTP 400 errors.

Reported in [unbound-force/dewey#55](https://github.com/unbound-force/dewey/issues/55).

## What Changes

## Capabilities

### New Capabilities

_None._

### Modified Capabilities

- `OllamaEmbedder.Embed()`: Wraps the single input string in a `[]string` before passing to `doEmbed()`, ensuring the JSON body always contains `"input": ["text"]` (array form).

### Removed Capabilities

_None._

## Impact

- **`embed/embed.go`** — One-line change in `Embed()` at line 131: `o.doEmbed(ctx, text)` becomes `o.doEmbed(ctx, []string{text})`.
- **`embed/embed_test.go`** — Add assertion in `TestOllamaEmbedder_Embed` verifying that the wire format sends an array, not a bare string.
- **Callers unaffected** — `vault/parse_export.go` (single-chunk fallback), `tools/semantic.go` (query embedding) call `Embed()` and benefit automatically.
- **`EmbedBatch()` unaffected** — already sends `[]string`.
- **Backward compatible** — Ollama accepts `[]string` for single inputs; no behavior change for working setups.

## Constitution Alignment

Assessed against the Unbound Force org constitution.

### I. Autonomous Collaboration

**Assessment**: N/A

This is an internal bug fix to JSON serialization. No change to artifact-based communication or MCP tool interfaces.

### II. Composability First

**Assessment**: PASS

Dewey remains independently installable. The fix improves compatibility with a wider range of Ollama implementations and proxies without introducing new dependencies.

### III. Observable Quality

**Assessment**: N/A

No change to output formats or provenance metadata. The fix affects the wire format of an internal HTTP request.

### IV. Testability

**Assessment**: PASS

The fix includes a test assertion verifying the wire format, ensuring the array form is enforced at the unit test level. The embed client remains testable in isolation via `httptest`.
