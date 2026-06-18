## Context

The `OllamaEmbedder.Embed()` method in `embed/embed.go` passes a bare `string` to `doEmbed()`, which serializes it as `"input": "text"` in the JSON body sent to Ollama's `/api/embed` endpoint. While the Ollama API historically accepted both `string` and `[]string` for the `input` field, strict implementations (including the `uf ollama-proxy`) require the array form `"input": ["text"]`. The `EmbedBatch()` method already correctly sends `[]string`. The Vertex AI embedder also wraps single strings in arrays at `embed/vertex.go:119`.

## Goals / Non-Goals

### Goals

- Ensure `Embed()` always sends the array form `"input": ["text"]` to `/api/embed`
- Add test coverage verifying the wire format of single-text embed requests
- Maintain backward compatibility with all Ollama versions

### Non-Goals

- Changing the `doEmbed()` signature or `embedRequest.Input` type (keeping it minimal per user decision)
- Modifying `EmbedBatch()` (already correct)
- Changing the Vertex AI embedder (already correct)

## Decisions

**Wrap at the call site, not the type level.** The fix wraps `text` in `[]string{text}` at `embed/embed.go:131` rather than changing `doEmbed(input any)` to `doEmbed(input []string)` or narrowing the `embedRequest.Input` type from `any` to `[]string`. This is the minimal change that fixes the bug while preserving the existing type flexibility of `doEmbed()`.

**Mirror the Vertex pattern.** The Vertex embedder's `Embed()` method already delegates to `EmbedBatch(ctx, []string{text})` at `embed/vertex.go:119`. Our fix follows the same spirit — always send arrays — without requiring the Ollama embedder to restructure its call chain.

**Test at the wire level.** The test assertion validates the JSON wire format by checking that `req.Input` is `[]any` (how `encoding/json` unmarshals a JSON array into `any`), matching the existing assertion pattern in `TestOllamaEmbedder_EmbedBatch` at `embed/embed_test.go:74`.

## Risks / Trade-offs

**Minimal risk.** The Ollama `/api/embed` endpoint accepts `[]string` for single inputs identically to bare strings — the response format (`embeddings: [[...]]`) is the same. No behavioral change for any working setup. The only effect is that previously-broken strict Ollama implementations now work.
