# Delta Spec: explicit-supersession

<!--
  Requirement numbering allocation
  ================================
  Capability: explicit-supersession (prefix ES)
  Allocated to this change: ES-FR-001 .. ES-FR-013
  Reserved for future explicit-supersession changes:
    ES-FR-014 .. ES-FR-020
  Sibling capabilities (their requirements are NOT here):
    - serialize-supersession-writes (prefix SSW):
      SSW-FR-001 .. SSW-FR-004 live in
      openspec/changes/serialize-supersession-writes/specs/
      serialize-supersession-writes/spec.md and own the guarded
      write-path semantics (atomic claim, losing-writer outcome,
      guarded backup leg, durability through re-ingestion) this
      delta's ES-FR-007/008/009 restate in orchestration terms and
      reference by id.
    - learning-temporal-validity (prefix LTV): LTV-FR-001 lives in
      openspec/changes/learning-temporal-validity/specs/
      learning-temporal-validity/spec.md (and is restated by the
      serialize-supersession-writes delta); LTV-FR-002 .. LTV-FR-020
      are reserved. LTV-FR-001 owns the two-way supersession contract
      on `store_learning`; ES-FR-007 extends the same link semantics
      to two existing learnings.
    - knowledge-distillation (prefix KD): KD-FR-104 lives in this
      change's specs/knowledge-distillation/spec.md delta; KD-FR-101
      .. KD-FR-103 live in
      openspec/changes/curator-agent-human-gates/specs/
      knowledge-distillation/spec.md; KD-FR-001 .. KD-FR-099 are
      reserved for the pending dewey-distill change. The distill
      review-gate requirement moved out of this capability during
      spec review and now carries a KD id.

  Affected-requirements context
  -----------------------------
  This change supersedes 013-knowledge-compile FR-008's newer-wins
  temporal merge ("decision learnings get temporal merge (newer wins)")
  for the compile pipeline: declared supersession, not recency, is the
  replacement authority. The contracts at
  specs/013-knowledge-compile/contracts/compile-tool.md and
  contracts/lint-tool.md are annotated with the superseded wording so
  the older contract and this delta cannot be read as agreeing.
  The sibling knowledge-distillation delta records the other upstream
  conflict this change resolves (the dewey-distill "Prompt-Mode
  Fallback Compatibility" byte-compat clause).

  Requirement ids MUST stay inside the allocated range.
-->

## ADDED Requirements

### Requirement: Declared Supersession Authority

**ID**: ES-FR-001

The compile synthesis prompt MUST NOT instruct resolution of
contradictions by recency. A learning MUST be treated as replaced
only when a declared supersession link or validity says so — a
`supersedes` link, a `superseded_by` link, or `validity:
superseded`. An unresolved contradiction MUST be surfaced, not
silently resolved: the prompt MUST instruct that both positions
appear in an `## Open Conflicts` section, each with its citations.

#### Scenario: Declared link is the only replacement authority

- **GIVEN** a cluster where learning B declares `supersedes: A`
- **WHEN** compile builds the synthesis prompt
- **THEN** the prompt MUST present B as replacing A because of the
  declared link
- **AND** MUST NOT contain any instruction to resolve contradictions
  by favoring the newest learning

#### Scenario: Unconfirmed contradiction becomes an open conflict

- **GIVEN** a cluster holding two contradictory learnings with no
  declared supersession link between them
- **WHEN** compile builds the synthesis prompt
- **THEN** the prompt MUST instruct that both positions appear in an
  `## Open Conflicts` section, each with its citations
- **AND** MUST NOT instruct that either position is resolved

### Requirement: Contradiction Remediation Guidance

**ID**: ES-FR-002

The lint contradiction finding's remediation MUST point at the
declared-supersession workflow — compile's supersession proposals
and the `supersede_learning` tool — and MUST NOT claim that compile
resolves contradictions via temporal merge.

#### Scenario: Lint remediation points at declared supersession

- **GIVEN** lint emits a contradiction finding
- **WHEN** the finding is rendered
- **THEN** its remediation MUST reference the declared-supersession
  workflow
- **AND** MUST NOT direct the user to resolve contradictions via
  temporal merge

### Requirement: Temporal Metadata in the Compile Input

**ID**: ES-FR-003

Compile MUST parse `validity`, `supersedes`, and `superseded_by`
from each learning's page properties into its learning entries, and
the synthesis prompt MUST render them per learning. Learnings whose
properties carry no temporal keys MUST render with no temporal
lines.

Decoded values MUST be normalized and shape-validated at the decode
boundary before they are rendered into a prompt: string values are
trimmed, `supersedes` and `superseded_by` are normalized with the
shared identity normalizer, `validity` MUST be one of the known
tokens, and the two link values MUST match the learning-identity
grammar. A value that fails validation, or that is not a string,
MUST be treated as absent for rendering and MUST never be emitted
into the prompt, so a hostile or corrupt property value cannot
inject prompt text.

#### Scenario: Learning with temporal fields

- **GIVEN** a learning whose properties carry `validity: superseded`
  and `superseded_by: auth-20261101T000000-alice`
- **WHEN** compile builds its learning entries and the synthesis
  prompt
- **THEN** the entry MUST carry both values
- **AND** the prompt MUST render them for that learning

#### Scenario: Legacy learning without temporal fields

- **GIVEN** a learning whose properties carry no temporal keys
- **WHEN** compile builds the synthesis prompt
- **THEN** that learning MUST render without temporal lines

#### Scenario: Malformed temporal value is withheld from the prompt

- **GIVEN** a learning whose properties carry a non-string
  `superseded_by`, an unrecognized `validity` token, or a
  `supersedes` value that does not match the identity grammar
- **WHEN** compile builds the synthesis prompt
- **THEN** the offending value MUST be treated as absent
- **AND** MUST NOT appear in the prompt

### Requirement: Per-Claim Citations

**ID**: ES-FR-004

The synthesis prompt MUST require every factual claim in the
synthesized article to end with a bracketed citation of the learning
identity — or comma-separated identities — the claim derives from.
The prompt MUST show the citation form and state that bracketed text
which is not a learning identity is ignored.

#### Scenario: Prompt requires per-claim citations

- **GIVEN** any cluster of learnings
- **WHEN** compile builds the synthesis prompt
- **THEN** the prompt MUST instruct that each factual claim ends with
  a bracketed citation naming the learning identity or identities it
  derives from
- **AND** MUST show the single-identity and comma-separated forms

### Requirement: Supersession Proposals Section

**ID**: ES-FR-005

The synthesis prompt MUST permit the synthesized article to end with
a `## Supersession Proposals` section in a strict machine-parseable
format — one entry per proposed pair, each carrying successor,
target, and rationale — and MUST state that a proposal is justified
only when one learning clearly replaces another, never by recency
alone. Compile MUST parse and strip the section before storing:
stored articles MUST NOT contain it.

#### Scenario: Section stripped from the stored article

- **GIVEN** a synthesized article ending with a `## Supersession
  Proposals` section
- **WHEN** compile processes the article
- **THEN** the stored article MUST NOT contain the section

#### Scenario: No proposals section

- **GIVEN** a synthesized article with no `## Supersession
  Proposals` section
- **WHEN** compile processes the article
- **THEN** the article MUST be stored unchanged by this requirement

### Requirement: Proposal Validation and Reporting

**ID**: ES-FR-006

A proposal entry is valid when its successor and target both resolve
to the article's source learnings, are distinct, and its rationale
is non-empty. Each valid entry MUST ride the compile result as a
`proposed_supersessions` item carrying successor, target, rationale,
and topic. Invalid entries MUST be dropped and reported without
failing the article write.

#### Scenario: Valid proposal rides the compile result

- **GIVEN** a proposals section whose entry names two of the
  article's source learnings with a rationale
- **WHEN** compile processes the article
- **THEN** the compile result MUST carry the proposal with its
  successor, target, rationale, and topic

#### Scenario: Invalid entries dropped, article still written

- **GIVEN** a proposals section whose entries name identities
  outside the article's sources, repeat one learning as both
  successor and target, or omit the rationale
- **WHEN** compile processes the article
- **THEN** the invalid entries MUST be dropped and reported
- **AND** the article MUST still be written

### Requirement: Supersede Learning Tool

**ID**: ES-FR-007

Dewey MUST provide a `supersede_learning` MCP tool that links two
existing learnings, successor and target, through the same guarded
write paths as `store_learning` supersession: the compare-and-swap
store claim (SSW-FR-001), the guarded markdown-backup leg
(SSW-FR-003), and one learning-properties lock hold across both
legs. The successor MUST gain `supersedes`; the target MUST gain
`superseded_by` and `validity: superseded` (LTV-FR-001 link
semantics, applied to two pre-existing learnings). The target MUST
NOT be deleted and its tier MUST NOT change.

Because both learnings already exist, the handler MUST re-read and
re-validate each learning's link state inside the lock hold before
writing — the same in-closure fast-fail the `store_learning` path
uses (SSW-FR-001, SSW-FR-002) — so a concurrent writer that changed
either side between validation and write is refused or routed to the
lost-claim outcome rather than overwritten.

#### Scenario: Confirm a proposal

- **GIVEN** learnings `auth-20261001T000000-alice` and
  `auth-20261101T000000-bob` exist with no supersession links
- **WHEN** `supersede_learning` is called with successor
  `auth-20261101T000000-bob` and target `auth-20261001T000000-alice`
- **THEN** the successor MUST record `supersedes` naming the target
  in the store and, when its backup exists, in its frontmatter
- **AND** the target MUST record the successor as `superseded_by`
  with `validity: superseded` in the store and, when its backup
  exists, in its frontmatter

#### Scenario: Target preserved in place

- **GIVEN** a successful `supersede_learning` call
- **WHEN** the target learning is read back
- **THEN** it MUST still exist
- **AND** its tier MUST be unchanged

#### Scenario: Link state re-validated inside the lock hold

- **GIVEN** a `supersede_learning` call that passed validation while
  the target recorded no `superseded_by`
- **WHEN** another writer records a successor on the target before
  this call's write
- **THEN** this call MUST NOT overwrite the recorded successor
- **AND** MUST report the lost-claim outcome

### Requirement: Supersede Learning Rejections

**ID**: ES-FR-008

The tool MUST refuse with an error result and no writes when either
identity names no learning, when successor and target are the same
learning, when the target already records a `superseded_by`, when
the successor already declares a `supersedes` naming a different
learning, or when following declared supersession links from either
learning reaches the other. The cycle walk MUST carry a visited set
or an explicit hop bound so it terminates on a pre-existing cycle; a
pre-existing cycle that already contains the two learnings is not a
reason to hang.

#### Scenario: Cycle refused

- **GIVEN** learning B already declares `supersedes: A`
- **WHEN** `supersede_learning` is called with successor A and
  target B
- **THEN** the tool MUST return an error result reporting the cycle
- **AND** neither learning MUST be written

#### Scenario: Pre-existing cycle terminates and is refused

- **GIVEN** a chain that already cycles through learnings A, B, and
  C, with neither A nor B the requested target in the other direction
- **WHEN** `supersede_learning` is called with successor A and
  target some unrelated learning D
- **THEN** the cycle walk MUST terminate via its visited set or hop
  bound
- **AND** the tool MUST return a result without writing either
  learning

#### Scenario: Target already superseded

- **GIVEN** the target already records a `superseded_by`
- **WHEN** `supersede_learning` is called naming that target
- **THEN** the tool MUST return an error result
- **AND** neither learning MUST be written

### Requirement: Supersede Learning Partial-Failure Warning

**ID**: ES-FR-009

A `supersede_learning` call whose store claim does not land, or
whose file leg fails after the store leg lands, MUST report
`supersedes_warning` naming the outcome, matching `store_learning`
semantics (SSW-FR-002).

#### Scenario: Lost claim reports the winner

- **GIVEN** a concurrent call has already recorded a successor on
  the target
- **WHEN** `supersede_learning` runs against the same target
- **THEN** the result MUST carry `supersedes_warning` naming the
  recorded winner
- **AND** the recorded successor MUST NOT be overwritten

### Requirement: Citation Coverage Check

**ID**: ES-FR-010

One shared pure helper MUST verify citation coverage in both
directions over an article's claim-bearing body and its declared
source learnings: forward — every bracketed citation resolves to a
declared source; backward — every declared source is cited at least
once.

A citation token is a bracketed group whose entire comma-separated
content matches the learning-identity grammar (a tag, an
`YYYYMMDDTHHMMSS` timestamp, and an author, joined by hyphens). A
bracketed group that does not match that grammar is not a citation
and MUST be ignored by both directions — markdown link text,
reference-style link labels, numeric footnotes, checkbox brackets,
double-bracket wikilinks, empty brackets, and prose in brackets are
all explicitly out of scope, so ordinary markdown never reads as an
unresolved citation.

Backward coverage MUST account for superseded sources: a source
recorded as superseded (a `superseded_by` or `validity: superseded`)
is accounted for when it is cited in the claim-bearing body or named
in the article's auto-generated `## History` table or an in-body
mention. For a source that is not superseded, the History table does
not satisfy backward coverage.

The frontmatter `sources` list and any `## Supersession Proposals`
section MUST NOT count as citations. Compile MUST report per-article
coverage for every article it writes.

#### Scenario: Unresolved citation reported

- **GIVEN** a compiled article whose body carries the citation
  `[auth-20261101T000000-al]` while its sources declare
  `auth-20261101T000000-alice`
- **WHEN** coverage is checked
- **THEN** the result MUST report the citation as unresolved

#### Scenario: Uncited source reported

- **GIVEN** a compiled article that declares
  `auth-20261001T000000-alice` among its sources but no bracketed
  citation in its body references it
- **WHEN** coverage is checked
- **THEN** the result MUST report that source as uncited

#### Scenario: Non-citation bracket forms are ignored

- **GIVEN** an article body carrying `[the docs](https://example.test)`,
  `[ref-label][target]`, `[1]`, `[x]`, `[[wiki-link]]`, `[]`, and
  `[see the notes]`
- **WHEN** coverage is checked
- **THEN** none of those groups MUST be reported as an unresolved
  citation

#### Scenario: Superseded source accounted for by History

- **GIVEN** an article that declares a source whose learning records
  `validity: superseded`, with no in-body citation for it but a row
  naming it in the `## History` table
- **WHEN** coverage is checked
- **THEN** that source MUST be accounted for
- **AND** MUST NOT be reported as uncited

#### Scenario: Compile reports per-article coverage

- **GIVEN** a synthesized article with an unresolved citation
- **WHEN** compile writes the article
- **THEN** the article's entry in the compile result MUST carry a
  coverage report naming the violation

### Requirement: Lint Citation Coverage

**ID**: ES-FR-011

Lint MUST gain a `citation_coverage` finding kind that re-runs the
same check over already-written compiled articles — hand-edited or
`store_compiled`-authored — reporting unresolved citations and
uncited sources by name; the kind MUST NOT be auto-repaired by
`fix`. A pre-existing compiled article written before this change
flooding the new kind is intended signal, not a false positive: the
remediation is to recompile the article (or correct its citations),
and records produced by this change MUST NOT inherit that backlog.

#### Scenario: Lint re-runs the check over written articles

- **GIVEN** an already-written compiled article with a coverage
  violation
- **WHEN** lint runs
- **THEN** it MUST emit a `citation_coverage` finding naming the
  article and the unresolved citations or uncited sources

#### Scenario: Legacy article is intended signal

- **GIVEN** a compiled article written before this change whose body
  carries no per-claim citations
- **WHEN** lint runs
- **THEN** it MUST report the article's uncited sources
- **AND** `fix` MUST NOT remove the finding

### Requirement: Store-Compiled Source Validation

**ID**: ES-FR-012

`store_compiled` MUST validate its declared `sources`: every entry
MUST map to an existing learning, and input naming no known learning
MUST return an error result naming the unresolved sources without
writing a page or file.

#### Scenario: Unknown source refused

- **GIVEN** `store_compiled` input declaring a source that names no
  existing learning
- **WHEN** the tool is called
- **THEN** it MUST return an error result naming the unresolved
  source
- **AND** no page or file MUST be written

### Requirement: Store-Compiled Coverage and Proposals

**ID**: ES-FR-013

`store_compiled` MUST verify and report citation coverage like
compile's, and a `## Supersession Proposals` section in the
submitted content MUST be parsed, validated, stripped, and returned
as `proposed_supersessions` exactly as compile does, so the
prompt-mode path surfaces proposals too.

#### Scenario: Coverage reported and proposals surfaced

- **GIVEN** `store_compiled` input whose content carries per-claim
  citations and ends with a `## Supersession Proposals` section
- **WHEN** the tool is called
- **THEN** the stored article MUST NOT contain the section
- **AND** the result MUST report coverage and carry the valid
  proposals

## MODIFIED Requirements

_None._ This capability has no archived main spec; every requirement
above is additive.

## REMOVED Requirements

_None._
