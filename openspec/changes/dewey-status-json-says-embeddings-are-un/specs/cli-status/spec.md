# Delta Spec: cli-status

<!--
  Requirement numbering allocation
  ================================
  Capability: cli-status (prefix STA)
  Allocated to dewey-status-json-says-embeddings-are-un: STA-FR-001 .. STA-FR-003
  Reserved for future changes: STA-FR-004 .. STA-FR-099
  Custody: the `dewey status` command's externally observable output. The
  speckit 001 contract (specs/001-core-implementation/contracts/cli-commands.md)
  already requires status to report the embedding model and availability; this
  change wires the two `--json` fields (and the shared model value) to real
  data. The MCP `health` tool's contract (mcp-tools.md) is untouched.
-->

## ADDED Requirements

### Requirement: `dewey status` resolves the embedding model exactly as serve does

**ID**: STA-FR-001

The `dewey status` command MUST report, in both `--json` and text output, the
embedding model that `dewey serve` would resolve for the same vault. The
value MUST come from `embed.ReadEmbeddingConfig` — never from line-based
`config.yaml` parsing — and the `readEmbeddingModel` helper MUST be removed.

#### Scenario: Scaffolded config comment is stripped

- **GIVEN** a vault whose `.uf/dewey/config.yaml` was written by `dewey init`
  and contains
  `model: granite-embedding:30m  # Override with DEWEY_EMBEDDING_MODEL env var`
- **AND** no env var or global config overrides it
- **WHEN** the user runs `dewey status --json`
- **THEN** `embeddingModel` MUST be exactly `granite-embedding:30m`

#### Scenario: `DEWEY_EMBEDDING_MODEL` overrides config

- **GIVEN** a vault whose `config.yaml` sets `embedding.model: config-model`
- **AND** `DEWEY_EMBEDDING_MODEL=env-model` is set
- **WHEN** the user runs `dewey status --json`
- **THEN** `embeddingModel` MUST be `env-model`

#### Scenario: No config file anywhere

- **GIVEN** a vault with `.uf/dewey/` but no per-vault `config.yaml`
- **AND** no global config exists and `DEWEY_EMBEDDING_MODEL` is unset
- **WHEN** the user runs `dewey status --json`
- **THEN** `embeddingModel` MUST be `granite-embedding:30m` (the serve
  default), not the empty string

#### Scenario: Global config supplies the model

- **GIVEN** a vault whose per-vault `config.yaml` sets neither
  `embedding.provider` nor `embedding.model`
- **AND** the global `~/.config/dewey/config.yaml` sets
  `embedding.model: global-model`
- **WHEN** the user runs `dewey status --json`
- **THEN** `embeddingModel` MUST be `global-model`

#### Scenario: Text output shows the same model

- **GIVEN** the scaffolded-config vault from the first scenario
- **WHEN** the user runs `dewey status` (no `--json`)
- **THEN** the `Model:` line MUST show `granite-embedding:30m` with no
  comment text

### Requirement: `embeddingAvailable` reports a real availability probe

**ID**: STA-FR-002

The `dewey status` command MUST set `embeddingAvailable` from the configured
embedder's `Available()` method, constructed via
`embed.NewEmbedderFromConfig` as the MCP `health` tool does. A provider that
cannot be constructed MUST yield `false` without failing the command. The
probe MUST be diagnostic-only (no auto-start of Ollama or any subprocess) and
time-bounded so an unreachable provider cannot hang the command.

#### Scenario: Provider serving the model

- **GIVEN** a vault whose `config.yaml` points the embedding endpoint at a
  server that lists the configured model as available
- **WHEN** the user runs `dewey status --json`
- **THEN** `embeddingAvailable` MUST be `true`

#### Scenario: Provider unreachable

- **GIVEN** a vault whose configured embedding endpoint is unreachable
  (nothing listens there)
- **WHEN** the user runs `dewey status --json`
- **THEN** the command MUST complete successfully (exit 0)
- **AND** `embeddingAvailable` MUST be `false`

#### Scenario: Model not served

- **GIVEN** a vault whose configured embedding endpoint responds but does
  not list the configured model
- **WHEN** the user runs `dewey status --json`
- **THEN** `embeddingAvailable` MUST be `false`

#### Scenario: Misconfigured provider

- **GIVEN** a vault whose `config.yaml` selects the `vertex` provider
  without a `project`
- **WHEN** the user runs `dewey status --json`
- **THEN** the command MUST complete successfully
- **AND** `embeddingAvailable` MUST be `false`

### Requirement: `dewey status --json` key set is unchanged

**ID**: STA-FR-003

The `--json` output object MUST keep exactly the key set it had before this
change — `path`, `pages`, `blocks`, `embeddings`, `embeddingModel`,
`embeddingAvailable`, `embeddingCoverage`, `sources`, and `trust` (when
present). No keys are added, renamed, or removed; only the values of
`embeddingModel` and `embeddingAvailable` are corrected.

#### Scenario: Key set stable

- **GIVEN** any initialized vault
- **WHEN** the user runs `dewey status --json`
- **THEN** the parsed object MUST contain `embeddingModel` and
  `embeddingAvailable` alongside the pre-existing keys
- **AND** no key other than the corrected pair's values differs in meaning
  from before this change
