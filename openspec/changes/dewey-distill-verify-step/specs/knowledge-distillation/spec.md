# Delta Spec: knowledge-distillation

<!--
  Requirement numbering allocation
  ================================
  Capability: knowledge-distillation (prefix KD)
  Allocated to this change: KD-FR-104 .. KD-FR-107
  Reserved for future knowledge-distillation changes from this
  repository's distill work: KD-FR-108 .. KD-FR-199
  Reserved for the pending dewey-distill change's archive-time
  numbering: KD-FR-001 .. KD-FR-099 — that change's requirements
    currently carry names, not ids, in
    openspec/changes/dewey-distill/specs/knowledge-distillation/
    spec.md ("Gated Distill Workflow" and siblings).
  Allocated to curator-agent-human-gates: KD-FR-101 .. KD-FR-103
    (curator binding, scaffold parity, human-gated questions) in
    openspec/changes/curator-agent-human-gates/specs/
    knowledge-distillation/spec.md.
  Sibling capabilities:
    - knowledge-verification (prefix KV): KV-FR-001 .. KV-FR-005
      own the `verify_candidates` tool contract this delta's
      command-side requirements call.
    - curator-agent (prefix CA): CA-FR-009 .. CA-FR-010 add
      `verify_candidates` to the curator allowlist.
  The capability's main spec is not archived yet: its existing
  pipeline contract is the named requirement "Gated Distill
  Workflow" in the dewey-distill change. Because no target spec
  exists in openspec/specs/, this delta MUST use ADDED
  requirements — a MODIFIED operation would be refused at archive
  time. These requirements add the deterministic verify step
  alongside the existing gated-workflow requirement rather than
  restating it. Requirement ids MUST stay inside the allocated
  range. Archive-time reconciliation: if the dewey-distill change
  archives first, the step order inside the archived "Gated
  Distill Workflow" text is reconciled with KD-FR-104's insertion
  point (verify between compile/curate and the review gate) so the
  archived pipeline reads coherently; the ids never collide.
-->

## ADDED Requirements

### Requirement: Deterministic Verify Step

**ID**: KD-FR-104

The `/dewey-distill` command MUST run a verify step between the
compile/curate step and the review gate. The step MUST be a single
`verify_candidates` MCP tool call whose scope is the run's
candidate queue: the tags the human approved at the scope gate of
the "Gated Distill Workflow" requirement (dewey-distill change),
or the pages the run produced across `learning/`, `compiled/`, and
`knowledge/`, using one selector as the tool's
`pages`/`tags` contract requires. The step MUST NOT reimplement anchor
checks in prose and MUST NOT shell out. On a `checked` pass it MUST
present each candidate's page name, tier, evidence locators, and
verification result. The verify step is informational and pre-gate
only.

#### Scenario: Verify runs after compile and before review
- **GIVEN** an approved scope and completed compile/curate
- **WHEN** the command reaches the verify step
- **THEN** it MUST call `verify_candidates` before presenting the
  review gate
- **AND** the review gate MUST NOT open before the call returns

#### Scenario: Scope passed to the tool
- **GIVEN** the human approved the `auth` tag at the scope gate
- **WHEN** the verify step runs
- **THEN** the tool call's scope MUST reflect the approved
  candidates
- **AND** the command MUST NOT hand-check anchors itself

#### Scenario: Checked pass presented per candidate
- **GIVEN** a `checked` verify result
- **WHEN** the command presents the verification
- **THEN** every candidate MUST be shown with its page name, tier,
  evidence locators, and verification result

### Requirement: Verification at the Review Gate

**ID**: KD-FR-105

The review gate MUST present the verification result for every
candidate before asking the human what to promote. Each candidate
MUST be shown with its identity, tier, evidence locators, and one
of `anchors_ok`, `drifted`, `missing`, `no_anchors`, or `skipped`
with its reason; every `skipped` entry — candidate or pass — MUST
be shown with its reason. The presentation MUST state code
correspondence only: it MUST NOT phrase `anchors_ok` as an
endorsement, a recommendation, or a pre-selection. The gate
question MUST remain the existing
`HUMAN_GATE:` review question, and nothing MUST be promoted
without explicit human selection.

#### Scenario: Results shown before the question
- **GIVEN** a verify result with `anchors_ok` and `drifted`
  candidates
- **WHEN** the review gate is presented
- **THEN** each candidate MUST show its result and evidence
- **AND** the question MUST follow the presented results

#### Scenario: Skipped entries carry their reason
- **GIVEN** a verify result containing a `skipped` candidate
  (for example, a requested page that no longer resolves)
- **WHEN** the review gate is presented
- **THEN** that candidate MUST be shown with its skip reason
- **AND** the presentation MUST NOT phrase any result as an
  endorsement or a recommendation to promote

#### Scenario: Unselected page not promoted
- **GIVEN** a human selects a subset of the eligible draft pages
- **WHEN** the command calls `promote`
- **THEN** only the selected pages MUST be promoted

#### Scenario: Gate marker preserved
- **GIVEN** the tracked command file
- **WHEN** its review-gate section is inspected
- **THEN** the question MUST still begin with the `HUMAN_GATE:`
  marker token

### Requirement: Degraded Verification at the Gate

**ID**: KD-FR-106

When verification is `skipped` or `failed`, the command MUST state
plainly at the review gate that verification was skipped or failed
and why, MUST NOT present the candidates as verified clean, and
MUST NOT substitute a hand-rolled check. The review gate MUST still
run so the human can decide with the information available.

#### Scenario: Skipped pass
- **GIVEN** the vault is not a git repository
- **WHEN** the review gate is presented
- **THEN** the command MUST say verification was skipped and why
- **AND** MUST NOT label any candidate as verified clean

#### Scenario: Failed pass
- **GIVEN** the verify tool returns a `failed` pass
- **WHEN** the review gate is presented
- **THEN** the command MUST relay the failure reason
- **AND** MUST NOT claim the candidates were checked

#### Scenario: Gate still runs
- **GIVEN** a skipped or failed verification
- **WHEN** the command reaches the review gate
- **THEN** the human MUST still be able to select pages to promote

### Requirement: Verify Never Promotes

**ID**: KD-FR-107

The verify step MUST NOT change a tier, a validity, or page
content, MUST NOT call `promote`, and MUST NOT pre-select pages for
the human. Every tier transition MUST still require the human's
review-gate selection and the subsequent `promote` call.

#### Scenario: Verify leaves tiers unchanged
- **GIVEN** draft candidates
- **WHEN** the verify step runs
- **THEN** every candidate's tier MUST remain `draft`

#### Scenario: No automatic promotion
- **GIVEN** a verify result where every candidate is `anchors_ok`
- **WHEN** the verify step completes
- **THEN** `promote` MUST NOT be called

#### Scenario: Human selection required
- **GIVEN** a human selects no pages at the review gate
- **WHEN** the command finishes
- **THEN** no page's tier MUST change

## MODIFIED Requirements

None.

## REMOVED Requirements

None.
