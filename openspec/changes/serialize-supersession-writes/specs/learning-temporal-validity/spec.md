<!--
Requirement numbering: this change allocates LTV-FR-001 for the
Two-Way Supersession requirement of the learning-temporal-validity
capability; LTV-FR-002 through LTV-FR-020 are reserved for future
changes to the same capability. The capability's other requirements
(As-Of Pinning, Evidence and Validity Inputs, Round-Trip Through
Re-Ingestion) live, un-numbered, in
openspec/changes/learning-temporal-validity/specs/
learning-temporal-validity.md; capabilities with a main spec live
under openspec/specs/.
-->
## ADDED Requirements

_None._

## MODIFIED Requirements

### Requirement: Two-Way Supersession

Requirement LTV-FR-001: write-time-enforced two-way supersession.

`store_learning` MUST accept optional `supersedes`: an identity or
`learning/<identity>` page name of an existing learning that has no
`superseded_by`. The new learning MUST record `supersedes`. The
target MUST gain `superseded_by` (the new identity) and
`validity: superseded` in the store — but only when this call's
supersession claim lands. When its markdown backup exists, the
frontmatter MUST record the same successor and validity, subject to
the guarded replace of serialize-supersession-writes SSW-FR-003: a
backup that already records a different successor stays unchanged.

The one-successor invariant MUST be enforced at write time. The store
claim MUST take effect only while the target records no
`superseded_by`, and the markdown backup leg MUST refuse to replace a
recorded successor that differs from the one being written. A call
whose claim does not land MUST keep its learning with its `supersedes`
claim intact, MUST NOT overwrite the recorded successor, and MUST
report `supersedes_warning` naming the winning successor. The target
MUST NOT be deleted and its tier MUST NOT change.

Previously: the target gained `superseded_by` and `validity:
superseded` only against the validation-time store snapshot, so two
writers that both passed validation could overwrite each other and
silently fork the one-successor chain with no `supersedes_warning`;
the markdown backup leg replaced the key unconditionally.

#### Scenario: Supersede an existing learning
- **GIVEN** learning `auth-20261101T000000-alice` exists with no
  `superseded_by`
- **WHEN** a learning is stored with
  `supersedes: "auth-20261101T000000-alice"`
- **THEN** the new learning MUST record that `supersedes`
- **AND** the target's properties MUST contain the new identity as
  `superseded_by` and `validity: superseded`
- **AND** when the target's markdown backup exists, its frontmatter
  MUST record the same successor and validity

#### Scenario: Concurrent losing claim
- **GIVEN** two `store_learning` calls superseding the same target
  both pass validation before either writes
- **WHEN** each call's write legs run and the first claim lands
  before the second
- **THEN** exactly one successor MUST remain recorded on the target
- **AND** the losing call MUST keep its learning with its `supersedes`
  claim intact and MUST report `supersedes_warning` naming the winner

#### Scenario: Unknown or already-superseded target
- **GIVEN** `supersedes` names no learning, or a learning that
  already has `superseded_by` at validation time
- **WHEN** a learning is stored
- **THEN** the tool MUST return an error result
- **AND** the tool MUST NOT write a page or file

## REMOVED Requirements

_None._
