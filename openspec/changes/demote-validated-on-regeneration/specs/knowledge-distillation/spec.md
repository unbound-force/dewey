<!--
  Requirement numbering allocation
  ================================
  Capability: knowledge-distillation (prefix KD)
  Allocated to this change: KD-FR-108
  Reserved for future changes to this capability:
  KD-FR-109 .. KD-FR-199
  Sibling capabilities:
    - validated-tier-content-binding (prefix VTB): VTB-FR-001 .. VTB-FR-005
      in this change's specs/validated-tier-content-binding/spec.md
      own the store content-hash rule (VTB-FR-001), the compiled-page
      hash recording (VTB-FR-002), the compile write-tier (VTB-FR-003),
      the curate re-index tier (VTB-FR-004), and promotion exclusivity
      (VTB-FR-005). VTB-FR-003 owns the compile write-path tier;
      this delta owns the compile tool's observable recompile contract
      (result fields, frontmatter, retention carve-outs). Do not
      restate one in the other.
    - learning-temporal-validity (prefix LTV) and
      serialize-supersession-writes (prefix SSW) own supersession
      links between learnings; this capability never edits or
      removes superseded knowledge (epic #153 append-only rule).
  The capability's main spec is not archived yet: its existing
  pipeline contract lives in openspec/changes/dewey-distill/specs/
  knowledge-distillation/spec.md as named requirements ("Server-Side
  Synthesis Wiring" and siblings). That change's requirements
  currently carry names, not ids. At archive time the dewey-distill
  change supplies KD-FR-001 .. KD-FR-099. Because no target spec
  exists in openspec/specs/, this delta MUST use ADDED requirements —
  a MODIFIED operation would be refused at archive time. This
  requirement adds the demote-on-regeneration contract that
  reconciles with (and replaces at archive time) the "Provenance
  and tier preservation" scenario of that pending requirement,
  flipping it to demote-on-regeneration. Archive-time reconciliation
  MUST preserve this flip; the dewey-distill baseline prose
  "recompiled article keeps tier validated" MUST NOT survive into
  the archived main spec.
  Requirement ids MUST stay inside the allocated range.
-->

## ADDED Requirements

### Requirement: Compiled Article Tier Demotes on Regeneration

**ID**: KD-FR-108

This requirement adds the demote-on-regeneration contract that
replaces the "Provenance and tier preservation" scenario of the
pending Server-Side Synthesis Wiring requirement at archive time.
Every article `compile` produces MUST be written with tier `draft`
(VTB-FR-003); preservation of a `validated` tier across regeneration
is exclusively the store rule's decision (VTB-FR-001): byte-identical
article bytes keep `validated` on hash equality, changed bytes land
at `draft`. Provenance recording (`compiled_by` in frontmatter,
page properties, and result) is unchanged. The failed-topic,
total-failure, and empty-collection retention scenarios are untouched
(no write occurs, so no demotion occurs).

#### Scenario: Recompiling a validated article with changed content demotes to draft

- **GIVEN** a compiled page with tier `validated` for tag `auth`
  and stored learnings for that tag
- **WHEN** `compile` regenerates the article and the produced
  article bytes differ from the stored article bytes
- **THEN** the compiled page's tier MUST be `draft` after the write
- **AND** the compile result's per-article `tier` field MUST report
  `draft`
- **AND** the regenerated article frontmatter MUST record
  `tier: draft`
- **AND** the page properties and result MUST still record the
  synthesizing model as `compiled_by`

#### Scenario: Byte-identical regeneration keeps validated

- **GIVEN** a compiled page with tier `validated` for tag `auth`
  and stored learnings for that tag
- **WHEN** `compile` regenerates the article and the produced
  article bytes are byte-identical to the stored article bytes
- **THEN** the compiled page's tier MUST remain `validated` after
  the write (the store rule VTB-FR-001 preserves it on hash
  equality) — the page row is the authority on the effective tier
- **AND** the compile result's per-article `tier` field MUST report
  the write tier `draft` (a write receipt; the effective tier lives
  on the page row)
- **AND** the regenerated article frontmatter MUST record
  `tier: draft` (the write tier compile wrote; the store row holds
  the preserved `validated`)

#### Scenario: A newly compiled topic is draft

- **GIVEN** no existing compiled page for a tag
- **WHEN** `compile` produces an article for it
- **THEN** the compiled page's tier MUST be `draft`

#### Scenario: Retention carve-outs are untouched

- **GIVEN** a `compile` run that fails for a topic, fails for every
  topic, or clears an empty collection (`retained_validated` path)
- **WHEN** the run completes
- **THEN** no existing compiled page MUST be demoted by this
  requirement — the validated article keeps its tier as before,
  because no write to that tag occurs
