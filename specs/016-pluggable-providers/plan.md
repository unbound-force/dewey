# Implementation Plan: Pluggable Providers

## Overview

Replace hardcoded Ollama construction with a provider-based configuration system. Add Vertex AI implementations for both `Embedder` and `Synthesizer` interfaces. Add `store_compiled` MCP tool. Add global config fallback.

## Design Decisions

### D1: Configuration Schema
Extend `config.yaml` with `embedding.provider` and `synthesis` sections. Ollama remains default. Vertex AI is the first cloud provider. Global config at `~/.config/dewey/config.yaml` provides defaults.

### D2: Provider Factory Functions
One factory per capability in its respective package (`embed.NewEmbedderFromConfig`, `llm.NewSynthesizerFromConfig`). ProviderConfig struct duplicated across packages to avoid cross-package coupling (Composability First).

### D3: Vertex AI Embedder
Direct REST calls to `{region}-aiplatform.googleapis.com/v1/.../models/{model}:predict`. OAuth via `golang.org/x/oauth2/google`. Pure Go, no CGO.

### D4: Vertex AI Synthesizer
Direct REST calls to `{region}-aiplatform.googleapis.com/v1/.../models/{model}:rawPredict`. Anthropic Messages format. 120-second timeout matching OllamaSynthesizer. Max tokens hardcoded at 4096.

### D5: `store_compiled` MCP Tool
Persists agent-synthesized articles as compiled pages. Tag validation prevents path traversal. Overwrites existing articles for the same tag.

### D6: OAuth Credential Management
Both Vertex providers use `golang.org/x/oauth2/google.FindDefaultCredentials` independently. No shared auth layer — the oauth2 library handles token caching internally.

### D7: Config Precedence Asymmetry
Embedding: env vars override config (backward compat with existing DEWEY_EMBEDDING_* vars). Synthesis: env vars are fallback only (new config, no legacy override behavior needed).

## Constitution Alignment

- **Composability First**: PASS — each provider is standalone, no mandatory dependencies
- **Autonomous Collaboration**: PASS — all communication via MCP tools
- **Observable Quality**: PASS — ModelID provenance, compiled_by tracking
- **Testability**: PASS — httptest isolation, no real API calls in tests
- **Local-Only Processing**: PASS (amended) — cloud providers are opt-in, Ollama is default
