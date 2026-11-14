<!--
  Requirement numbering allocation
  ================================
  Capability: validated-tier-content-binding (prefix VTB)
  Allocated to this change: VTB-FR-001 .. VTB-FR-005
  Reserved for future changes to this capability:
  VTB-FR-006 .. VTB-FR-099
  Sibling capabilities:
    - knowledge-distillation (prefix KD): KD-FR-108 in this
      change's sibling delta owns the compile tool's observable
      recompile contract (result fields, frontmatter, retention
      carve-outs). VTB-FR-001 owns the store rule; VTB-FR-003
      owns the compile write path's tier. Do not restate one in
      the other.
    - learning-temporal-validity (prefix LTV) and
      serialize-supersession-writes (prefix SSW) own supersession
      links between learnings; this capability never edits or
      removes superseded knowledge (epic #153 append-only rule).
-->

## ADDED Requirements

### Requirement: Content-Bound Validated Preservation

**ID**: VTB-FR-001

`Store.UpdatePage` MUST preserve a stored `validated` tier only
when the incoming content hash equals the stored content hash;
when the hashes differ, the caller-supplied tier MUST apply. Two
empty content hashes MUST compare as equal, so writers that do not
track content hashes retain the pre-change never-demote behavior.
The rule MUST NOT change tier handling for pages whose stored tier
is not `validated` — the caller-supplied tier applies exactly as
before, on hash match and hash change alike.

#### Scenario: Changed content demotes a validated page

- **GIVEN** a store page with tier `validated` and content hash
  `H1`
- **WHEN** `UpdatePage` writes that page with content hash `H2`
  (different from `H1`) and tier `draft`
- **THEN** the stored tier MUST be `draft`

#### Scenario: Identical content preserves a validated page

- **GIVEN** a store page with tier `validated` and content hash
  `H1`
- **WHEN** `UpdatePage` writes that page with content hash `H1`
  and tier `draft`
- **THEN** the stored tier MUST remain `validated`

#### Scenario: The promote race stays closed

- **GIVEN** a caller reads a `validated` page and writes it back
  with its content hash unchanged (for example a metadata write
  racing a promotion, the read-then-write window the existing
  `UpdatePage` comment describes)
- **WHEN** the write lands
- **THEN** the stored tier MUST remain `validated`
- **AND** the existing race-regression test
  (`TestUpdatePage_PreservesValidatedTier`) MUST pass unmodified

#### Scenario: Hash-less writers keep the pre-change behavior

- **GIVEN** a store page with tier `validated` and an empty
  content hash
- **WHEN** `UpdatePage` writes that page with an empty content
  hash and tier `draft`
- **THEN** the stored tier MUST remain `validated`

#### Scenario: Non-validated tiers are unaffected

- **GIVEN** a store page with tier `draft` (or `curated`)
- **WHEN** `UpdatePage` writes that page with any content hash and
  a caller-supplied tier
- **THEN** the stored tier MUST be the caller-supplied tier

### Requirement: Compiled Page Content Hashes

**ID**: VTB-FR-002

The `compile` and `store_compiled` write paths MUST set the
persisted compiled page's `content_hash` to the hex encoding of
the first 8 bytes of the SHA-256 digest of the stored article
bytes — the same truncated-hash convention the curation re-index
and learning re-ingestion paths use — so the VTB-FR-001 rule can
distinguish changed from unchanged compiled content. The hashed
bytes MUST be exactly the article content persisted to the store
and written to the article file.

#### Scenario: Compile records the article hash

- **GIVEN** `compile` persists an article for a tag
- **WHEN** the compiled page row is read back
- **THEN** its `content_hash` MUST be non-empty and MUST equal the
  hex encoding of the first 8 bytes of the SHA-256 digest of the
  article bytes written to `.uf/dewey/compiled/{tag}.md`

#### Scenario: store_compiled records the article hash

- **GIVEN** `store_compiled` persists an agent-synthesized article
- **WHEN** the compiled page row is read back
- **THEN** its `content_hash` MUST be non-empty and MUST equal the
  hex encoding of the first 8 bytes of the SHA-256 digest of the
  stored article content

#### Scenario: Identical bytes produce identical hashes

- **GIVEN** two article contents that are byte-identical
- **WHEN** each is hashed by the compiled write paths
- **THEN** both page rows MUST carry the same `content_hash`

### Requirement: Compile Regeneration Writes Draft

**ID**: VTB-FR-003

Every article produced by `compile` MUST be written with tier
`draft`. `compile` MUST NOT look up the existing compiled page's
tier to carry it onto regenerated content. Preservation of a
`validated` tier across regeneration is exclusively the store
rule's decision (VTB-FR-001): byte-identical article bytes keep
`validated`; changed bytes land at `draft`.

#### Scenario: Recompiling a validated article with changed content

- **GIVEN** a compiled page with tier `validated` for tag `auth`,
  and stored learnings for that tag
- **WHEN** `compile` regenerates the article and the produced
  article bytes differ from the stored article bytes
- **THEN** the compiled page's tier MUST be `draft` after the
  write
- **AND** the compile result's per-article `tier` field MUST
  report `draft`
- **AND** the regenerated article frontmatter MUST record
  `tier: draft`

#### Scenario: A newly compiled topic is draft

- **GIVEN** no existing compiled page for a tag
- **WHEN** `compile` produces an article for it
- **THEN** the compiled page's tier MUST be `draft`

### Requirement: Curate Re-index Writes Curated on Change

**ID**: VTB-FR-004

When `curate` re-indexes a knowledge file whose content hash
differs from the stored page's, the page MUST land at tier
`curated` — the same tier a fresh knowledge-page insert receives.
A knowledge file whose content hash is unchanged MUST be skipped
without any page write, leaving the stored tier (including
`validated`) untouched.

#### Scenario: Changed validated knowledge file re-indexes as curated

- **GIVEN** a knowledge page with tier `validated` and content
  hash `H1`, and the knowledge file on disk now hashes to `H2`
- **WHEN** `curate` re-indexes the knowledge store
- **THEN** the page's tier MUST be `curated`

#### Scenario: Unchanged validated knowledge file is skipped

- **GIVEN** a knowledge page with tier `validated` whose file on
  disk still hashes to the stored content hash
- **WHEN** `curate` re-indexes the knowledge store
- **THEN** the page MUST be skipped without a write
- **AND** the stored tier MUST remain `validated`

### Requirement: Promotion Exclusivity Preserved

**ID**: VTB-FR-005

This capability MUST NOT add any path onto tier `validated`. The
content-hash rule only preserves `validated` or applies a
caller-supplied tier; it MUST NOT elevate a page whose stored tier
is not `validated`. The machine write paths this capability
governs (`compile`, `store_compiled`, `curate` re-index) MUST NOT
write tier `validated`. The human promotion gate remains the only
path that elevates agent-generated content, through its existing
conditional update (`UPDATE pages SET tier = 'validated' ... WHERE
tier = 'draft'`), which this change MUST NOT alter.

#### Scenario: A machine write never elevates to validated

- **GIVEN** a compiled or knowledge page with tier `draft`
- **WHEN** any machine write path in this capability writes it
  with changed or unchanged content
- **THEN** the stored tier MUST NOT become `validated`

#### Scenario: Promote still elevates drafts

- **GIVEN** a compiled page with tier `draft`
- **WHEN** the promotion gate promotes it
- **THEN** the stored tier MUST become `validated`
- **AND** the promotion SQL MUST remain the unchanged conditional
  update on `tier = 'draft'`
