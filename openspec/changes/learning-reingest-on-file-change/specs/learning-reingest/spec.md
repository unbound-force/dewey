# Delta Spec: learning-reingest

<!--
  Requirement numbering allocation
  ================================
  Capability: learning-reingest (prefix LR)
  Allocated to learning-reingest-on-file-change: LR-FR-001 .. LR-FR-002
  Reserved for future changes: LR-FR-003 .. LR-FR-099
  Custody: extends the file-backed-learning recovery behavior of
  speckit spec 015 (FR-003, reIngestLearnings); does not change it.
-->

## ADDED Requirements

### Requirement: Learnings are re-ingested when their file changes

**ID**: LR-FR-001

During `dewey serve` startup re-ingestion, when a learning file's
`learning/<identity>` page exists in the store and the page's stored
content hash differs from the hash of the file's current body —
including when the stored hash is empty — Dewey MUST re-ingest the
learning from the file: delete the page's existing blocks, update the
page row with the file-derived content hash, properties, category and
source identifiers, re-persist the blocks, and regenerate embeddings
when the embedder is available. The re-ingested page's tier MUST be
`draft`.

#### Scenario: A changed body is re-ingested after a pull

- **GIVEN** an indexed learning whose file body has been edited on
  disk (for example by a `git pull` that fast-forwards the repository)
- **WHEN** `dewey serve` starts and startup re-ingestion runs
- **THEN** the page's blocks reflect the new body
- **AND** the page's stored content hash equals the hash of the new
  body
- **AND** embeddings for the page are regenerated when the embedder is
  available

#### Scenario: A missing page is still recovered

- **GIVEN** a learning file in `.uf/dewey/learnings/` whose
  `learning/<identity>` page does not exist in the store
- **WHEN** startup re-ingestion runs
- **THEN** the learning is inserted, persisted and embedded as before
  this change

#### Scenario: A forged tier is ignored on the update path

- **GIVEN** a learning file whose body changed and whose frontmatter
  claims `tier: validated`
- **WHEN** the learning is re-ingested
- **THEN** the page's tier is `draft`
- **AND** the ignored claim is logged as a warning

#### Scenario: A drifted store tier converges to draft

- **GIVEN** a learning page whose stored tier is not `draft` (a state
  no tool can produce — learning pages are never promotable) and whose
  file is unchanged
- **WHEN** startup re-ingestion runs
- **THEN** the page row's tier is refreshed to `draft` without block or
  embedding work

### Requirement: Unchanged learning files stay cheap at startup

**ID**: LR-FR-002

When a learning page's stored content hash matches the hash of its
file's current body, startup re-ingestion MUST NOT re-persist blocks,
MUST NOT regenerate embeddings, and MUST NOT count the learning in its
return value; the page row's properties, category and tier MUST be
refreshed in place from the file when they differ, without block work.
The hash comparison MUST use the file-body hash the insertion path
uses, and `store_learning` MUST compute its content hash over the same
bytes, so unchanged files never re-embed.

#### Scenario: An unchanged file does no block or embedding work

- **GIVEN** an indexed learning whose file is unchanged since it was
  indexed
- **WHEN** `dewey serve` starts again and startup re-ingestion runs
- **THEN** no blocks are re-persisted for the learning
- **AND** no embeddings are generated for it
- **AND** the re-ingestion count is 0

#### Scenario: A frontmatter-only change refreshes properties without block work

- **GIVEN** an indexed learning whose file body is unchanged but whose
  frontmatter changed on disk (for example `validity:
  stale-unverified` arrived via a pull)
- **WHEN** startup re-ingestion runs
- **THEN** the page's properties reflect the new frontmatter
- **AND** its blocks and embeddings are untouched

#### Scenario: A learning stored by the tool matches on the next startup

- **GIVEN** a learning stored through the `store_learning` MCP tool
- **WHEN** `dewey serve` starts and startup re-ingestion runs over the
  unchanged file the tool wrote
- **THEN** the stored hash matches the file-derived hash and the
  learning is not re-ingested

#### Scenario: A divergent stored hash converges exactly once

- **GIVEN** an indexed learning whose stored content hash disagrees
  with the hash derived from its unchanged file (a row written by an
  earlier version over different bytes, or with an empty hash)
- **WHEN** startup re-ingestion runs
- **THEN** the learning is re-ingested once and the stored hash becomes
  the file-derived hash
- **AND** every later startup over the same file performs no block or
  embedding work for it
