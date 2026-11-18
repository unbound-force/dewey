# Delta Spec: index-provenance

<!--
  Requirement numbering allocation
  ================================
  Capability: index-provenance (prefix IBP)
  Allocated to record-which-dewey-built-an-index-and-sa:
    IBP-FR-001 .. IBP-FR-005
  Reserved for future changes: IBP-FR-006 .. IBP-FR-099
  Custody: new capability. Settled design from the gate in epic
  unbound-force/dewey#226, decided in #224's comments (index format
  number, counts-as-zero, strictly-lower recommendation).
-->

## ADDED Requirements

### Requirement: Indexing passes record their build stamp

**ID**: IBP-FR-001

Every completed indexing pass over a persistent store MUST write two
keys to the existing `metadata` table: `dewey_version`, the resolved
version (IBP-FR-002) of the binary that ran the pass, and
`index_format`, the decimal value of that binary's `IndexFormat`
(IBP-FR-004). The stamp MUST be a metadata-only upsert: it MUST NOT
re-persist or regenerate any page, block, or embedding, and MUST NOT
change `schema_version` or any table shape. A pass that fails MUST NOT
update the keys.

#### Scenario: `dewey index` stamps after success

- **GIVEN** an initialized vault with a persistent store and at least
  one configured source
- **WHEN** `dewey index` completes successfully
- **THEN** `metadata` holds `dewey_version` equal to the binary's
  resolved version and `index_format` equal to its `IndexFormat`

#### Scenario: `dewey reindex` stamps after a clean rebuild

- **GIVEN** an initialized vault with an existing graph.db
- **WHEN** `dewey reindex` completes successfully
- **THEN** both keys hold the rebuilding binary's values

#### Scenario: The serve startup pass stamps once, after all passes

- **GIVEN** an initialized vault with a persistent store
- **WHEN** `dewey serve` startup indexing (learning re-ingestion,
  knowledge-store re-index, vault index, external-page load) completes
  successfully
- **THEN** both keys hold the serving binary's values
- **AND** the stamp was written after the passes completed, not before

#### Scenario: The MCP index and reindex tools stamp

- **GIVEN** a running `dewey serve` with a persistent store
- **WHEN** the `index` or `reindex` MCP tool completes an indexing
  operation
- **THEN** both keys hold the serving binary's values

#### Scenario: A failed pass leaves the previous stamp

- **GIVEN** a store whose `index_format` is `1`
- **WHEN** an indexing pass from a binary with `IndexFormat = 2` fails
  before completing
- **THEN** `index_format` still reads `1` and `dewey_version` is
  unchanged

#### Scenario: Stamping is metadata-only

- **GIVEN** a fully indexed store whose source files are unchanged
- **WHEN** an indexing pass runs and stamps
- **THEN** no blocks or embeddings are regenerated for those files
- **AND** `schema_version` and every table shape are unchanged

#### Scenario: In-memory mode skips stamping

- **GIVEN** a `dewey serve` running without a persistent store
- **WHEN** startup indexing completes
- **THEN** no stamp is written and startup behaves exactly as before

### Requirement: The binary resolves its version from ldflags or build info

**ID**: IBP-FR-002

Dewey MUST resolve its version in this order: the ldflags-injected
version when it is neither empty nor `dev`; otherwise
`debug.ReadBuildInfo().Main.Version` when it is neither empty nor
`(devel)`; otherwise `dev`. `dewey version` MUST print the resolved
version, and the serve startup log MUST report it. The resolved
version MUST be the value written to `dewey_version` (IBP-FR-001) and
MUST NOT be parsed for comparison anywhere.

#### Scenario: A release build uses the ldflags version

- **GIVEN** a binary built with the release ldflags version `3.9.0`
- **WHEN** the version is resolved
- **THEN** it reads `3.9.0`

#### Scenario: A mise `go:` install uses the build-info module version

- **GIVEN** a binary built with `go install <module>@v3.8.1` (ldflags
  version unset, so `dev`)
- **WHEN** the version is resolved
- **THEN** it reads `v3.8.1`

#### Scenario: A local build resolves to `dev`

- **GIVEN** a binary built with `go build` from a working copy (ldflags
  `dev`, build-info version `(devel)`)
- **WHEN** the version is resolved
- **THEN** it reads `dev`

#### Scenario: `dewey version` prints the resolved version

- **GIVEN** a mise `go:` install as above
- **WHEN** `dewey version` runs
- **THEN** the output is `v3.8.1`, not `dev`

### Requirement: Status reports index provenance and a reindex recommendation

**ID**: IBP-FR-003

`dewey status --json` MUST report four top-level fields:
`deweyVersion` (the stored `dewey_version`, empty when absent),
`indexFormat` (the stored `index_format`; a missing or non-decimal
value counts as `0`), `reindexRecommended` (true if and only if the
stored format is lower than this binary's `IndexFormat`), and
`reindexReasons` (the recommendation reasons, empty when none).
`dewey status` MUST NOT write either key.

#### Scenario: An older format is recommended

- **GIVEN** a store with `index_format` `1`
- **WHEN** `dewey status --json` runs on a binary with
  `IndexFormat = 2`
- **THEN** `indexFormat` is `1`, `reindexRecommended` is `true`, and
  `reindexReasons` is
  `["index format 1, this Dewey writes 2"]`

#### Scenario: A pre-tracking index counts as format 0

- **GIVEN** a store built by Dewey 3.8.0 (no `index_format` key)
- **WHEN** `dewey status --json` runs on a binary with
  `IndexFormat = 1`
- **THEN** `indexFormat` is `0` and `reindexRecommended` is `true`

#### Scenario: A current format is not recommended

- **GIVEN** a store with `index_format` equal to the binary's
  `IndexFormat`
- **WHEN** `dewey status --json` runs
- **THEN** `reindexRecommended` is `false` and `reindexReasons` is
  empty

#### Scenario: A newer stored format is not recommended

- **GIVEN** a store with `index_format` `3`
- **WHEN** `dewey status --json` runs on a binary with
  `IndexFormat = 2`
- **THEN** `reindexRecommended` is `false`

#### Scenario: An unparseable stored format counts as 0

- **GIVEN** a store whose `index_format` value is `banana`
- **WHEN** `dewey status --json` runs on a binary with
  `IndexFormat = 1`
- **THEN** `indexFormat` is `0` and `reindexRecommended` is `true`

#### Scenario: No database, no recommendation

- **GIVEN** a vault with `.uf/dewey/` but no graph.db
- **WHEN** `dewey status --json` runs
- **THEN** `deweyVersion` is empty, `indexFormat` is `0`, and
  `reindexRecommended` is `false`

#### Scenario: Status never stamps

- **GIVEN** a store with no `index_format` key
- **WHEN** `dewey status --json` runs
- **THEN** neither `index_format` nor `dewey_version` exists in
  `metadata` afterwards

### Requirement: An IndexFormat constant gates indexing-behavior changes

**ID**: IBP-FR-004

The code MUST carry an `IndexFormat` constant whose initial value is
`1`. Any change to what indexing stores MUST bump `IndexFormat`.
AGENTS.md MUST document the bump rule in its PR checklist so review
catches a missed bump.

#### Scenario: The constant exists

- **GIVEN** the codebase
- **WHEN** `IndexFormat` is read
- **THEN** it is defined with value `1` and a doc comment stating the
  bump rule

#### Scenario: AGENTS.md carries the bump rule

- **GIVEN** AGENTS.md
- **WHEN** a reviewer checks the PR checklist
- **THEN** it requires bumping `IndexFormat` for any PR that changes
  what indexing stores

### Requirement: Human-readable status shows the index provenance

**ID**: IBP-FR-005

The human-readable `dewey status` output MUST show the recorded index
version and, when a reindex is recommended, the reason.

#### Scenario: Text output shows version and recommendation

- **GIVEN** a store with `dewey_version` `v3.8.1` and `index_format`
  `1`, checked by a binary with `IndexFormat = 2`
- **WHEN** `dewey status` runs without `--json`
- **THEN** the output shows `v3.8.1` as the recorded version and the
  reindex reason `index format 1, this Dewey writes 2`
