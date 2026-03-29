## ADDED Requirements

### Requirement: dewey doctor Command

The `dewey doctor` command MUST check all Dewey prerequisites and report pass/fail for each with actionable fix instructions.

#### Scenario: All prerequisites met
- **GIVEN** `.dewey/` exists, `graph.db` has pages, Ollama is running, and the embedding model is available
- **WHEN** the user runs `dewey doctor`
- **THEN** all checks show pass status

#### Scenario: Ollama not running
- **GIVEN** Ollama is not running on the configured endpoint
- **WHEN** the user runs `dewey doctor`
- **THEN** the Ollama check fails with instructions to start Ollama

#### Scenario: Model not pulled
- **GIVEN** Ollama is running but the embedding model is not pulled
- **WHEN** the user runs `dewey doctor`
- **THEN** the model check fails with the exact `ollama pull` command to run

#### Scenario: dewey not initialized
- **GIVEN** no `.dewey/` directory exists in the vault path
- **WHEN** the user runs `dewey doctor`
- **THEN** the init check fails with `dewey init` as the fix command

### Requirement: --no-embeddings Flag

The `dewey serve`, `dewey index`, and root `dewey` commands MUST accept a `--no-embeddings` flag that skips embedding model creation and availability checks.

#### Scenario: Serve with --no-embeddings
- **GIVEN** Ollama is not running
- **WHEN** the user runs `dewey serve --no-embeddings --vault .`
- **THEN** the server starts successfully without error, and semantic search tools return "embeddings disabled"

#### Scenario: Index with --no-embeddings
- **GIVEN** Ollama is not running
- **WHEN** the user runs `dewey index --no-embeddings`
- **THEN** indexing completes successfully, blocks and links are persisted, but no embeddings are generated

## MODIFIED Requirements

### Requirement: Embedding Model Availability Error

When the embedding model is unavailable and `--no-embeddings` is NOT set, `dewey serve` and `dewey index` MUST fail with a hard error containing: the model name, the endpoint, the fix command (`ollama pull <model>`), and how to skip (`--no-embeddings`).

Previously: Logged a WARN and continued with silent degradation.

#### Scenario: Serve without Ollama and without --no-embeddings
- **GIVEN** Ollama is not running and `--no-embeddings` is not set
- **WHEN** the user runs `dewey serve --vault .`
- **THEN** the command exits with an error containing `ollama pull granite-embedding:30m`

#### Scenario: Index without model and without --no-embeddings
- **GIVEN** the embedding model is not pulled and `--no-embeddings` is not set
- **WHEN** the user runs `dewey index`
- **THEN** the command exits with an error containing `ollama pull granite-embedding:30m`

## REMOVED Requirements

_None_
