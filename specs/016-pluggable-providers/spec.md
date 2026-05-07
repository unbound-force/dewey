# Feature Specification: Pluggable Embedding & Synthesis Providers

**Feature Branch**: `016-pluggable-providers`
**Created**: 2026-05-05
**Status**: Implementation Complete
**Input**: Make Dewey's embedding and synthesis LLM backends configurable — Ollama (default) and Vertex AI, with global config fallback

## Constitution Amendment: Local-Only Processing

The constitution's Development Standards state: *"No data MUST leave the developer's machine for core functionality."* This change introduces Vertex AI providers that send data to Google Cloud when explicitly configured by the user.

**Interpretation**: Cloud providers are an **opt-in extension**, not core functionality. Ollama remains the default. All core capabilities (keyword search, graph navigation, indexing, MCP tools) continue to work without any cloud dependency. The user makes a conscious choice to enable cloud backends via `config.yaml`. This is consistent with the constitution's own language: "for core functionality" and "The embedding model MUST be configurable."

**AGENTS.md line 81** should be updated from "No data MUST leave the developer's machine" to "No data leaves the developer's machine by default. Cloud providers (Vertex AI) are opt-in via config."

---

## User Scenarios & Testing *(mandatory)*

### User Story 1 — Configurable Synthesis Provider (Priority: P1)

A developer runs `dewey compile` on a machine without a GPU. The default Ollama model (`llama3.2:3b`) produces low-quality synthesis in 6.5 minutes. The developer adds `synthesis.provider: vertex` to their config, and compilation now uses Claude via Vertex AI — producing a high-quality article in 9 seconds.

**Why this priority**: Synthesis quality directly affects the value of compiled knowledge. Local models produce generic output; frontier models produce genuine synthesis.

**Independent Test**: Configure Vertex AI synthesis in config.yaml. Run `dewey compile` with stored learnings. Verify the configured model is used and the output is persisted.

**Acceptance Scenarios**:

1. **Given** `config.yaml` contains `synthesis.provider: vertex`, `synthesis.model: claude-sonnet-4-6`, `synthesis.project: my-project`, `synthesis.region: us-east5`, **When** `dewey compile` is run, **Then** Dewey uses VertexSynthesizer for compilation
2. **Given** `config.yaml` has no `synthesis` section but has `compile_model: llama3.2:3b`, **When** `dewey compile` is run, **Then** Dewey constructs an OllamaSynthesizer (backward compatibility)
3. **Given** no synthesis configuration exists, **When** `dewey compile` is run, **Then** Dewey operates in prompt-only mode (returns synthesis prompts without LLM)
4. **Given** an unknown provider is configured (`synthesis.provider: unsupported`), **When** the factory is called, **Then** it returns an error containing the unknown provider name

---

### User Story 2 — Configurable Embedding Provider (Priority: P1)

A developer wants to use Vertex AI embedding models instead of local Ollama. They add `embedding.provider: vertex` to their config, and `dewey index` generates embeddings via the Vertex AI prediction API.

**Why this priority**: Enables teams without Ollama to use semantic search, and allows higher-quality embedding models.

**Independent Test**: Configure Vertex AI embedding in config.yaml. Run `dewey index`. Verify embeddings are generated via the configured provider.

**Acceptance Scenarios**:

1. **Given** `config.yaml` contains `embedding.provider: vertex`, `embedding.model: text-embedding-005`, `embedding.project: my-project`, `embedding.region: us-central1`, **When** `dewey index` is run, **Then** Dewey uses VertexEmbedder
2. **Given** `config.yaml` contains `embedding.model: granite-embedding:30m` without a `provider` field, **When** Dewey creates an embedder, **Then** it defaults to OllamaEmbedder (backward compatibility)
3. **Given** env vars `DEWEY_EMBEDDING_MODEL` and `DEWEY_EMBEDDING_ENDPOINT` are set, **When** Dewey reads embedding config, **Then** env vars override config file values for embedding
4. **Given** a VertexEmbedder with valid credentials, **When** `EmbedBatch(ctx, ["text1", "text2"])` is called, **Then** it returns two float32 vectors in input order

---

### User Story 3 — Agent-Driven Compilation via `store_compiled` (Priority: P2)

An AI agent calls `compile` and receives synthesis prompts (nil synthesizer path). The agent synthesizes the article itself — it's already a frontier LLM. The agent then calls `store_compiled` to persist the result back to Dewey as a compiled article with provenance tracking.

**Why this priority**: Closes the loop on the existing nil-synthesizer MCP path. Agents can now compile knowledge without any local LLM.

**Independent Test**: Call `store_compiled` with a tag, content, sources, and model. Verify the article is persisted to filesystem and store.

**Acceptance Scenarios**:

1. **Given** an agent calls `store_compiled` with `tag: "auth"`, `content: "..."`, `sources: ["auth-1", "auth-3"]`, **Then** Dewey persists a page with `source_id: "compiled"`, `tier: "draft"`, and writes to `.uf/dewey/compiled/auth.md`
2. **Given** an agent provides `model: "claude-opus-4-6"`, **When** the article is persisted, **Then** frontmatter includes `compiled_by: claude-opus-4-6`
3. **Given** a compiled article for tag `auth` already exists, **When** `store_compiled` is called with tag `auth`, **Then** the existing article is overwritten
4. **Given** an empty `tag` field, **When** `store_compiled` is called, **Then** it returns an error

---

### User Story 4 — Global Config (Priority: P2)

A developer uses Vertex AI for synthesis across all projects. Instead of copying the same config to every vault, they create `~/.config/dewey/config.yaml` with their provider settings. Every vault inherits these defaults unless overridden.

**Why this priority**: Reduces configuration friction for teams using cloud providers.

**Independent Test**: Create a global config with Vertex synthesis. Run `dewey compile` in a vault with no local config. Verify the global config is used.

**Acceptance Scenarios**:

1. **Given** no per-vault config exists, **And** global config at `~/.config/dewey/config.yaml` has `synthesis.provider: vertex`, **When** `dewey compile` is run, **Then** Dewey uses the global Vertex config
2. **Given** per-vault config has `synthesis.provider: ollama`, **And** global config has `synthesis.provider: vertex`, **When** `dewey compile` is run, **Then** per-vault config wins
3. **Given** `XDG_CONFIG_HOME` is set, **When** Dewey reads global config, **Then** it looks in `$XDG_CONFIG_HOME/dewey/config.yaml`

---

## Config Precedence

Embedding config precedence (env vars override config):
1. Environment variables (`DEWEY_EMBEDDING_MODEL`, `DEWEY_EMBEDDING_ENDPOINT`)
2. Per-vault `.uf/dewey/config.yaml`
3. Global `~/.config/dewey/config.yaml`
4. Defaults (ollama, granite-embedding:30m, localhost:11434)

Synthesis config precedence (env vars are fallback only):
1. Per-vault `.uf/dewey/config.yaml` (`synthesis` section)
2. Per-vault legacy `compile_model` field
3. Global `~/.config/dewey/config.yaml`
4. Environment variable `DEWEY_GENERATION_MODEL`
5. No synthesizer (prompt-only mode)

**Note**: The asymmetry between embedding and synthesis precedence is intentional — embedding env vars have historically overridden config, while synthesis config was introduced in this spec and doesn't need backward-compatible env var override behavior.

---

## Tag Validation

The `store_compiled` tool's `tag` field MUST be validated to prevent path traversal. Tags MUST contain only alphanumeric characters, hyphens, and underscores. Tags containing path separators (`/`, `\`), relative path components (`..`), or null bytes MUST be rejected with a clear error.

---

## Functional Requirements

- **FR-001**: Dewey MUST support `ollama` and `vertex` as embedding providers, configurable via `config.yaml`
- **FR-002**: Dewey MUST support `ollama` and `vertex` as synthesis providers, configurable via `config.yaml`
- **FR-003**: Vertex providers MUST use `golang.org/x/oauth2/google` application-default credentials (no CGO)
- **FR-004**: Vertex providers MUST NOT store credentials in config files
- **FR-005**: A `store_compiled` MCP tool MUST allow agents to persist compiled articles with provenance
- **FR-006**: Global config at `~/.config/dewey/config.yaml` MUST provide defaults for all vaults
- **FR-007**: Per-vault config MUST override global config
- **FR-008**: All existing configs, env vars, and the nil-synthesizer path MUST continue to work
- **FR-009**: Factory functions `NewEmbedderFromConfig` and `NewSynthesizerFromConfig` MUST centralize provider construction
- **FR-010**: The `store_compiled` tag field MUST be validated against path traversal
- **FR-011**: Vertex providers MUST retry on HTTP 429 (Too Many Requests) with exponential backoff
- **FR-012**: Vertex providers MUST respect the `Retry-After` header when present in 429 responses
- **FR-013**: Vertex providers MUST retry up to 5 times before returning an error to the caller
