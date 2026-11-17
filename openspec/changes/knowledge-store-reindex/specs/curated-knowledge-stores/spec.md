# Delta Spec: curated-knowledge-stores

<!--
  Requirement numbering allocation
  ================================
  Capability: curated-knowledge-stores (prefix CKS)
  Allocated to knowledge-store-reindex: CKS-FR-001 .. CKS-FR-002
  Reserved for future changes: CKS-FR-003 .. CKS-FR-099
  Custody: extends spec 015 FR-027/FR-028 (auto-indexed knowledge
  stores, source_id knowledge-{store-name}); does not change them.
-->

## ADDED Requirements

### Requirement: Knowledge stores are re-indexed from their files at startup

**ID**: CKS-FR-001

During `dewey serve` startup indexing, after learnings are re-ingested
and before the vault is indexed, Dewey MUST index every store listed in
`.uf/dewey/knowledge-stores.yaml` from the markdown files in its store
directory, using the same page naming (`knowledge/{store}/{file}`),
`source_id` (`knowledge-{store}`) and tier derivation as curation's
auto-index. `_index.md` and dot-files MUST NOT be indexed. A missing
config file MUST be a no-op; an invalid config MUST be logged and MUST
NOT abort startup indexing.

#### Scenario: Pages return after the database is rebuilt

- **GIVEN** a vault whose `knowledge-stores.yaml` lists store `team-kb`
  (with or without sources) and whose store directory holds
  `claim-a.md`, `claim-b.md` and `_index.md`
- **AND** a graph.db with no `knowledge/team-kb/*` pages
- **WHEN** `dewey serve` starts and background indexing completes
- **THEN** pages `knowledge/team-kb/claim-a` and
  `knowledge/team-kb/claim-b` exist with source `knowledge-team-kb`
- **AND** no page exists for `_index.md`

#### Scenario: Unchanged pages are not re-embedded

- **GIVEN** the pages above already indexed with matching content
- **WHEN** the server starts again
- **THEN** no blocks or embeddings are regenerated for them

### Requirement: Knowledge-store pages carry their frontmatter as properties

**ID**: CKS-FR-002

When a knowledge-store file is indexed, its parsed frontmatter MUST be
stored as the page's properties JSON, encoded as the vault indexer
encodes it. A page whose content is unchanged but whose stored
properties differ (including empty properties written by earlier
versions) MUST have its properties refreshed without re-persisting its
blocks.

#### Scenario: Validity is visible to retrieval

- **GIVEN** a store file with frontmatter `validity: stale-unverified`
  and `as_of: org/repo@abc123`
- **WHEN** it is indexed
- **THEN** the page's validity reads `stale-unverified` and its as_of
  reads `org/repo@abc123` for validity-aware semantic search

#### Scenario: Earlier rows are backfilled

- **GIVEN** an indexed store page whose stored properties are empty and
  whose file is unchanged
- **WHEN** the store is indexed again
- **THEN** the page's properties hold the file's frontmatter
- **AND** its blocks are unchanged
