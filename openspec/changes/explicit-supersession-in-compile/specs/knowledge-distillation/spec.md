# Delta Spec: knowledge-distillation

<!--
  Requirement numbering allocation
  ================================
  Capability: knowledge-distillation (prefix KD)
  Allocated to this change: KD-FR-104
  Reserved for future knowledge-distillation changes:
    KD-FR-105 .. KD-FR-199
  Ranges held by sibling changes (their requirements are NOT here):
    - KD-FR-101 .. KD-FR-103 belong to curator-agent-human-gates
      (agent binding, scaffold parity, human-gated gate markers) in
      openspec/changes/curator-agent-human-gates/specs/
      knowledge-distillation/spec.md.
    - KD-FR-001 .. KD-FR-099 are reserved for the pending
      dewey-distill change's archive-time numbering; that change's
      requirements currently carry names, not ids.
  Sibling capabilities:
    - explicit-supersession (prefix ES): ES-FR-001 .. ES-FR-013 live
      in this change's specs/explicit-supersession/spec.md and own
      the compile-side policy, the `supersede_learning` tool, and the
      coverage check this delta's gate presents.
    - learning-temporal-validity (prefix LTV): LTV-FR-001 owns the
      two-way supersession contract the confirmed pairs are recorded
      through.
  The capability's main spec is not archived yet (no target exists
  in openspec/specs/knowledge-distillation/), so this delta MUST use
  ADDED requirements; a MODIFIED operation would be refused at
  archive time.

  Affected-requirements context
  -----------------------------
  This requirement explicitly supersedes the dewey-distill
  "Prompt-Mode Fallback Compatibility" requirement's byte-compat
  clause for compile: that clause requires non-empty learning
  collections to return `prompts_ready` payloads "byte-compatible
  with the pre-change behavior". ES-FR-001 through ES-FR-005 change
  the synthesis prompt (temporal lines, Open Conflicts, citation
  instructions, the proposals section), so the prompt bytes
  necessarily differ. The payload's shape and the cluster/prompt
  fields remain compatible; only the prompt text changes, and the
  prompt-mode hand-off to `store_compiled` is unchanged. Every other
  clause of that requirement and of "Gated Distill Workflow" stands.
  Requirement ids MUST stay inside the allocated range.
-->

## ADDED Requirements

### Requirement: Distill Review Gate Integration

**ID**: KD-FR-104

The `/dewey-distill` review gate MUST present proposed
supersessions — successor, target, rationale — and per-article
coverage findings next to the promote list. A supersession MUST be
recorded only after the human confirms it at the gate, by calling
the `supersede_learning` tool (ES-FR-007, whose link writes follow
LTV-FR-001) for each confirmed pair before promotion. A proposal the
human does not confirm MUST leave both learnings' links unchanged,
and nothing MAY be promoted without the existing gate approval.

The gate MUST NOT introduce a third marked question: the
confirmation rides the existing review-gate question, whose
`HUMAN_GATE:` marker stays reserved for the two mandatory gates
(KD-FR-103). Both copies of the command change together — the
tracked file and the byte-identical embedded copy (KD-FR-102).

#### Scenario: Gate presents proposals and coverage

- **GIVEN** a compile result carrying `proposed_supersessions` and
  per-article coverage
- **WHEN** the review gate runs
- **THEN** it MUST present each proposal's successor, target, and
  rationale and each article's coverage findings beside the promote
  list

#### Scenario: Confirmed supersessions applied before promotion

- **GIVEN** the human confirms one of the presented proposals
- **WHEN** the pipeline proceeds to promotion
- **THEN** `supersede_learning` MUST have been called for the
  confirmed pair before `promote`
- **AND** every unconfirmed proposal MUST leave both learnings
  unchanged

#### Scenario: Confirmed supersession apply failure aborts promotion

- **GIVEN** the human confirms a proposal
- **WHEN** the `supersede_learning` call returns an error result or
  a `supersedes_warning`
- **THEN** the pipeline MUST NOT call `promote`
- **AND** it MUST relay the failure verbatim, naming the successor,
  target, and the reported error or warning

#### Scenario: No third marked question

- **GIVEN** the tracked command file
- **WHEN** its marked question lines are counted by section
- **THEN** each mandatory gate section MUST still contain exactly one
  marker-prefixed line
- **AND** the supersession confirmation MUST NOT add another

## MODIFIED Requirements

_None._ No target spec exists in `openspec/specs/` for this
capability; the delta is additive, and the upstream requirement it
supersedes lives in the unarchived dewey-distill change.

## REMOVED Requirements

_None._
