# Delta Spec: learning-reingest

<!--
  Requirement numbering allocation
  ================================
  Capability: learning-reingest (prefix LR)
  Allocated to reindex-reingests-learnings: LR-FR-003
  Reserved for future changes: LR-FR-004 .. LR-FR-099
-->

## ADDED Requirements

### Requirement: Reindex restores file-backed pages

**ID**: LR-FR-003

After `dewey reindex` has indexed the configured sources, and before it
records the index-build stamp, it MUST re-ingest learnings from
`.uf/dewey/learnings/` and re-index every knowledge store configured in
`.uf/dewey/knowledge-stores.yaml`, with the same behavior as `dewey
serve` startup. A failure in either pass MUST be logged as a warning
and MUST NOT fail the command.

#### Scenario: Learnings and curated pages survive a reindex

- **GIVEN** a vault with a learning file and a configured knowledge
  store with one page file
- **WHEN** `dewey reindex --no-embeddings` completes
- **THEN** `graph.db` contains the `learning/<identity>` page with its
  frontmatter properties and blocks
- **AND** it contains the `knowledge/<store>/<page>` page under source
  `knowledge-<store>`
- **AND** the configured sources are indexed and the index-build stamp
  is recorded
