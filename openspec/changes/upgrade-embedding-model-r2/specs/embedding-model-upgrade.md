## ADDED Requirements

### Requirement: Configurable chunk character limit

The embedding pipeline MUST support a configurable maximum chunk character limit. The limit SHALL be resolved via the following precedence chain (highest to lowest):

1. `DEWEY_CHUNK_MAX_CHARS` environment variable
2. `embedding.max_chunk_chars` field in per-vault `config.yaml`
3. `embedding.max_chunk_chars` field in global `config.yaml`
4. Default value of `12288`

The configured value MUST be an integer greater than zero. Invalid values (non-positive integers, non-numeric strings) MUST be logged as a warning and MUST fall back to the default value of 12288.

#### Scenario: Invalid chunk limit from env var (non-numeric)

- **GIVEN** `DEWEY_CHUNK_MAX_CHARS=abc` is set in the environment
- **WHEN** the embedding pipeline resolves its configuration
- **THEN** a warning is logged and the default value of 12288 is used

#### Scenario: Zero chunk limit from env var

- **GIVEN** `DEWEY_CHUNK_MAX_CHARS=0` is set in the environment
- **WHEN** the embedding pipeline resolves its configuration
- **THEN** a warning is logged and the default value of 12288 is used

#### Scenario: Negative chunk limit from env var

- **GIVEN** `DEWEY_CHUNK_MAX_CHARS=-5` is set in the environment
- **WHEN** the embedding pipeline resolves its configuration
- **THEN** a warning is logged and the default value of 12288 is used

#### Scenario: Chunk limit from env var

- **GIVEN** `DEWEY_CHUNK_MAX_CHARS=2048` is set in the environment
- **WHEN** the embedding pipeline prepares a chunk
- **THEN** the chunk is truncated to at most 2048 characters

#### Scenario: Chunk limit from config file

- **GIVEN** `config.yaml` contains `embedding.max_chunk_chars: 4096` and no env var is set
- **WHEN** the embedding pipeline prepares a chunk
- **THEN** the chunk is truncated to at most 4096 characters

#### Scenario: Default chunk limit

- **GIVEN** no env var or config file sets `max_chunk_chars`
- **WHEN** the embedding pipeline prepares a chunk
- **THEN** the chunk is truncated to at most 12288 characters

### Requirement: PrepareChunk accepts configurable limit

The `PrepareChunk` function MUST accept a `maxChars int` parameter instead of using a package-level constant. All callers MUST pass the configured chunk limit.

#### Scenario: PrepareChunk with explicit limit

- **GIVEN** a block with 5000 characters of content
- **WHEN** `PrepareChunk` is called with `maxChars=3000`
- **THEN** the returned chunk is at most 3000 characters (rune-based truncation)

#### Scenario: PrepareChunk preserves context path

- **GIVEN** a block with a heading path and long content
- **WHEN** `PrepareChunk` is called with a limit shorter than context path + content
- **THEN** the context path prefix is preserved in the truncated result

#### Scenario: PrepareChunk with no truncation needed

- **GIVEN** a block with 100 characters of content
- **WHEN** `PrepareChunk` is called with `maxChars=12288`
- **THEN** the full content is returned without truncation

Note: When `maxChars` is smaller than the context path itself, the entire output (including the context path) is truncated to `maxChars` characters. The context path is best-effort preserved but is NOT exempt from truncation.

### Requirement: Doctor legacy model advisory

`dewey doctor` SHOULD display an informational note when the configured embedding model is `granite-embedding:30m`, advising the user that a newer model is available.

The note MUST NOT be a failure -- it SHALL be displayed as an informational marker.

#### Scenario: Doctor detects legacy model

- **GIVEN** the user's `config.yaml` specifies `model: granite-embedding:30m`
- **WHEN** `dewey doctor` runs the Embedding Layer checks
- **THEN** an informational note is displayed: the configured model has a newer replacement available, suggesting the user update their config and run `dewey reindex`

#### Scenario: Doctor with current model

- **GIVEN** the user's config specifies the R2 model or any non-legacy model
- **WHEN** `dewey doctor` runs the Embedding Layer checks
- **THEN** no legacy model advisory is displayed

## MODIFIED Requirements

### Requirement: Default embedding model

The default embedding model MUST be updated from `granite-embedding:30m` to the Granite Embedding R2 small model name once it is available on Ollama's library.

Previously: The default was `granite-embedding:30m` hardcoded in `embed/config.go:149` and the `dewey init` config template in `cli.go:251`.

**Guard**: If the R2 model is not yet available on Ollama when this change ships, the default MUST remain `granite-embedding:30m`. The configurable chunk limit ships independently.

#### Scenario: New installation gets R2 default

- **GIVEN** no `config.yaml` exists and no `DEWEY_EMBEDDING_MODEL` env var is set
- **WHEN** the embedding pipeline resolves its configuration
- **THEN** the model defaults to the R2 model name

#### Scenario: Existing config overrides default

- **GIVEN** `config.yaml` specifies `model: granite-embedding:30m`
- **WHEN** the embedding pipeline resolves its configuration
- **THEN** `granite-embedding:30m` is used (config takes precedence over default)

### Requirement: Init template reflects current default

The `dewey init` command MUST write a `config.yaml` template with the current default model name. The template MUST include a comment noting the model can be overridden via `DEWEY_EMBEDDING_MODEL`.

Previously: The template hardcoded `granite-embedding:30m` at `cli.go:251`.

#### Scenario: dewey init writes updated template

- **GIVEN** the user runs `dewey init` in a new vault
- **WHEN** the config.yaml is generated
- **THEN** the `embedding.model` field contains the current default model name

## REMOVED Requirements

None. All existing functionality is preserved. The `granite-embedding:30m` model continues to work when explicitly configured.

## Documentation Impact

The following files MUST be updated upon implementation:
- `README.md` -- Environment Variables table (add `DEWEY_CHUNK_MAX_CHARS`), Provider Configuration examples (add `max_chunk_chars`), Semantic Search Setup section (update model name and `ollama pull` command)
- `AGENTS.md` -- Provider Configuration section (add `max_chunk_chars`), Active Technologies, Environment Variables
- `.specify/memory/constitution.md` -- line 156 (update default model name parenthetical, PATCH version bump)
- Website (`unbound-force/website`) -- file a GitHub issue tracking the new env var, config field, default model change, and doctor advisory
