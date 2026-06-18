## ADDED Requirements

### Requirement: OLLAMA_HOST env var fallback

When resolving the Ollama endpoint, Dewey MUST check environment variables in
this order:
1. `DEWEY_EMBEDDING_ENDPOINT` (app-specific override)
2. `OLLAMA_HOST` (ecosystem standard)
3. Default constant `http://localhost:11434`

If `OLLAMA_HOST` contains a value without a URL scheme (e.g., `0.0.0.0:11434`),
Dewey MUST prepend `http://` to normalize it.

#### Scenario: OLLAMA_HOST is set, DEWEY_EMBEDDING_ENDPOINT is not

- **GIVEN** `OLLAMA_HOST=http://host.docker.internal:11435` is set
- **AND** `DEWEY_EMBEDDING_ENDPOINT` is not set
- **AND** no `config.yaml` `embedding.endpoint` is configured
- **WHEN** Dewey resolves the embedding endpoint
- **THEN** the resolved endpoint MUST be `http://host.docker.internal:11435`

#### Scenario: DEWEY_EMBEDDING_ENDPOINT takes precedence over OLLAMA_HOST

- **GIVEN** `DEWEY_EMBEDDING_ENDPOINT=http://localhost:11434` is set
- **AND** `OLLAMA_HOST=http://remote:11435` is set
- **WHEN** Dewey resolves the embedding endpoint
- **THEN** the resolved endpoint MUST be `http://localhost:11434`

#### Scenario: Neither env var is set

- **GIVEN** neither `DEWEY_EMBEDDING_ENDPOINT` nor `OLLAMA_HOST` is set
- **AND** no `config.yaml` `embedding.endpoint` is configured
- **WHEN** Dewey resolves the embedding endpoint
- **THEN** the resolved endpoint MUST be `http://localhost:11434`

#### Scenario: OLLAMA_HOST without scheme

- **GIVEN** `OLLAMA_HOST=0.0.0.0:11434` is set (no `http://` prefix)
- **AND** `DEWEY_EMBEDDING_ENDPOINT` is not set
- **WHEN** Dewey resolves the embedding endpoint
- **THEN** the resolved endpoint MUST be `http://0.0.0.0:11434`

#### Scenario: config.yaml endpoint takes precedence over OLLAMA_HOST

- **GIVEN** `config.yaml` sets `embedding.endpoint: http://config-host:11434`
- **AND** `OLLAMA_HOST=http://env-host:11435` is set
- **AND** `DEWEY_EMBEDDING_ENDPOINT` is not set
- **WHEN** Dewey resolves the embedding endpoint
- **THEN** the resolved endpoint MUST be `http://config-host:11434`

#### Scenario: OLLAMA_HOST with HTTPS scheme preserved

- **GIVEN** `OLLAMA_HOST=https://ollama.internal:11434` is set
- **AND** `DEWEY_EMBEDDING_ENDPOINT` is not set
- **WHEN** Dewey resolves the embedding endpoint
- **THEN** the resolved endpoint MUST be `https://ollama.internal:11434`

#### Scenario: OLLAMA_HOST empty string treated as unset

- **GIVEN** `OLLAMA_HOST=` is set (empty string)
- **AND** `DEWEY_EMBEDDING_ENDPOINT` is not set
- **AND** no `config.yaml` `embedding.endpoint` is configured
- **WHEN** Dewey resolves the embedding endpoint
- **THEN** the resolved endpoint MUST be `http://localhost:11434`

### Requirement: Default endpoint constant

The default Ollama endpoint value `http://localhost:11434` MUST be defined as a
single exported constant in the `embed` package. All code that references this
default MUST use the constant instead of hardcoding the string.

#### Scenario: Constant used across packages

- **GIVEN** the `embed.DefaultOllamaEndpoint` constant exists
- **WHEN** a developer searches for the string `http://localhost:11434` in Go source files
- **THEN** no hardcoded instances SHALL exist outside of the constant definition and test assertions

### Requirement: Graceful degradation on missing embedding model

When Ollama is reachable but the requested embedding model is not available,
Dewey MUST log a warning and continue in keyword-only mode instead of exiting
with a fatal error.

The warning MUST include:
- The model name that was not found
- The endpoint that was checked
- Instructions to pull the model (`ollama pull <model>`)
- A note that semantic search is unavailable

This requirement applies to both `dewey serve` (`main.go:initObsidianBackend`)
and `dewey index`/`dewey reindex` (`cli.go:createIndexEmbedder`).

#### Scenario: Model not pulled, dewey serve

- **GIVEN** Ollama is running at the resolved endpoint
- **AND** the embedding model `granite-embedding:30m` has not been pulled
- **WHEN** the user runs `dewey serve --vault .`
- **THEN** Dewey MUST log a warning containing "embedding model" and "not available"
- **AND** Dewey MUST start successfully in keyword-only mode
- **AND** semantic search MCP tools MUST return clear error messages indicating embeddings are unavailable

#### Scenario: Model not pulled, dewey index

- **GIVEN** Ollama is running at the resolved endpoint
- **AND** the embedding model has not been pulled
- **WHEN** the user runs `dewey index --vault .`
- **THEN** Dewey MUST log a warning containing the model name, endpoint, and `ollama pull` instructions
- **AND** Dewey MUST skip embedding generation
- **AND** the index MUST be built successfully with pages and blocks (but no embeddings)

#### Scenario: Model available, no behavior change

- **GIVEN** Ollama is running and the model is available
- **WHEN** the user runs `dewey serve --vault .`
- **THEN** Dewey MUST start with full embedding support (no behavior change)

## MODIFIED Requirements

### Requirement: dewey doctor endpoint resolution

Previously: `dewey doctor` reads `DEWEY_EMBEDDING_ENDPOINT` directly via
`os.Getenv()`, falling back to `http://localhost:11434`. This bypasses
`config.yaml` and the new `OLLAMA_HOST` fallback.

Modified: `dewey doctor` MUST use `embed.ReadEmbeddingConfig()` to resolve the
embedding endpoint and model, ensuring it reports the same values that
`dewey serve` and `dewey index` use.

#### Scenario: Doctor reports config.yaml endpoint

- **GIVEN** `config.yaml` sets `embedding.endpoint: http://custom:11434`
- **AND** no env vars override it
- **WHEN** the user runs `dewey doctor --vault .`
- **THEN** the Embedding Layer section MUST display `http://custom:11434`

#### Scenario: Doctor reports OLLAMA_HOST endpoint

- **GIVEN** `OLLAMA_HOST=http://remote:11435` is set
- **AND** `DEWEY_EMBEDDING_ENDPOINT` is not set
- **AND** no `config.yaml` `embedding.endpoint` is configured
- **WHEN** the user runs `dewey doctor --vault .`
- **THEN** the Embedding Layer section MUST display `http://remote:11435`

## REMOVED Requirements

None.
