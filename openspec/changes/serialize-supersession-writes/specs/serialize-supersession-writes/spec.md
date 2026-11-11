<!--
Requirement numbering: this change allocates SSW-FR-001 through
SSW-FR-004 for the serialize-supersession-writes capability;
SSW-FR-005 through SSW-FR-020 are reserved for future changes to the
same capability. The modified sibling capability
learning-temporal-validity keeps its requirements in its own delta
(openspec/changes/learning-temporal-validity/specs/
learning-temporal-validity.md); capabilities with a main spec live
under openspec/specs/.
-->
## ADDED Requirements

### Requirement: Atomic Same-Target Supersession Claim

Requirement SSW-FR-001: atomic same-target supersession claim.

The store leg MUST claim the target atomically: the write MUST take
effect only while the target records no `superseded_by`, and a losing
writer MUST NOT overwrite the recorded successor. The claim MUST be
enforced at write time inside the store operation.

A refused claim MUST disambiguate the recorded value with a comma-ok
type assertion, failing closed as a hard error when it is not a
string.

#### Scenario: Two concurrent supersessions of the same target

- **GIVEN** learning `auth-20261101T000000-alice` exists with no
  `superseded_by`
- **WHEN** two `store_learning` calls superseding it run concurrently
  and both pass validation
- **THEN** exactly one successor MUST be recorded as the target's
  `superseded_by` in the store
- **AND** the other call MUST NOT have overwritten it

#### Scenario: Target superseded between validation and write

- **GIVEN** a `store_learning` call passed validation against a target
  with no `superseded_by`
- **WHEN** another writer records a successor on that target before
  the first call's store leg runs
- **THEN** the first call's claim MUST NOT take effect and MUST NOT
  overwrite the recorded successor

#### Scenario: Wrong-typed recorded superseded_by fails closed

- **GIVEN** the target's `superseded_by` property holds a non-string
  JSON value (for example a number, boolean, object, or array)
- **WHEN** a call's claim is refused and the recorded value is
  disambiguated with a comma-ok type assertion
- **THEN** the call MUST report the store-leg failure warning
- **AND** the tool MUST NOT treat the target as unclaimed or report an
  empty successor identity

### Requirement: Losing Writer Outcome

Requirement SSW-FR-002: losing writer outcome.

A supersession call whose claim does not land MUST report
`supersedes_warning` naming the winning successor, and MUST keep its
learning with its `supersedes` claim intact. The target's tier and
lifecycle follow learning-temporal-validity LTV-FR-001, which owns
that outcome; this requirement adds only the warning. Sequential
supersession of an already-superseded target is LTV-FR-001's "Unknown
or already-superseded target" scenario.

#### Scenario: Concurrent loser is warned and kept

- **GIVEN** two concurrent `store_learning` calls superseding the same
  target, one of which claimed it
- **WHEN** the other call completes
- **THEN** its result MUST carry `supersedes_warning` naming the
  winning successor
- **AND** its learning MUST be stored, recording its own `supersedes`

### Requirement: Guarded Markdown Backup Leg

Requirement SSW-FR-003: guarded markdown backup leg.

Before replacing `superseded_by` in a target's markdown backup, the
file leg MUST re-read the backup and compare the recorded value
against the identity being written. It MUST leave the file unchanged,
reporting the refusal
through `supersedes_warning`, when the backup records a different
successor or an unparseable scalar. The file leg MUST NOT run when
the store claim did not land; a missing backup is not an error.

#### Scenario: Backup already names a different successor

- **GIVEN** the target's markdown backup records
  `superseded_by: "old-1"` and the store claim just landed for
  `new-1`
- **WHEN** the file leg runs
- **THEN** the backup MUST remain byte-for-byte unchanged
- **AND** the call MUST report `supersedes_warning` describing the
  refusal

#### Scenario: Backup records the prefixed form of the same successor

- **GIVEN** the target's markdown backup records
  `superseded_by: "learning/new-1"` and the store claim just landed
  for `new-1`
- **WHEN** the file leg normalizes both values with
  `supersedesIdentity` and compares them
- **THEN** the values MUST compare equal and the write MUST proceed

#### Scenario: Backup unclaimed or idempotent

- **GIVEN** the target's markdown backup records no `superseded_by`,
  an empty `superseded_by`, or one equal to the successor being
  written
- **WHEN** the file leg runs
- **THEN** the backup MUST record the successor as `superseded_by`
  and `validity: superseded`, with its body untouched

#### Scenario: Unparseable recorded value is refused

- **GIVEN** the target's markdown backup records a `superseded_by`
  scalar the line reader cannot parse
- **WHEN** the file leg runs
- **THEN** the backup MUST remain unchanged
- **AND** the call MUST report `supersedes_warning` describing the
  refusal

#### Scenario: No markdown backup

- **GIVEN** the target learning has no markdown backup file
- **WHEN** the file leg runs
- **THEN** it MUST NOT be an error, and the tool MUST NOT create a file

#### Scenario: Lint fix interleaved with a supersession

- **GIVEN** lint's anchor fix and a supersession both target the same
  learning, and the supersession's store claim and file write land
  first
- **WHEN** lint's store and file legs run under the shared
  `learningPropsMu` hold
- **THEN** lint's store leg MUST skip the learning because it now
  records `validity: superseded`
- **AND** lint's file leg MUST NOT overwrite the backup's
  `superseded_by` or `validity: superseded`

### Requirement: Durability Through Re-Ingestion

Requirement SSW-FR-004: durability through re-ingestion.

After a supersession whose file leg wrote the target's backup, the
recorded chain MUST survive deletion of the SQLite store:
re-ingesting from the markdown backups MUST restore exactly one
successor, recording the winning successor as `superseded_by` and
`validity: superseded`, matching the guarded backup. A supersession
whose file leg refused is excluded; the durable backup keeps the
successor it already recorded.

#### Scenario: Chain survives graph.db loss

- **GIVEN** a supersession whose file leg wrote the target's backup
  completed under the guarded write legs
- **WHEN** the SQLite store is deleted and `reIngestLearnings` runs
  over the learnings directory
- **THEN** the restored target MUST record the winning successor as
  `superseded_by` with `validity: superseded`
- **AND** the restored winning successor MUST record its `supersedes`
- **AND** a restored learning MUST NOT record a `superseded_by`
  naming a different successor for that target

#### Scenario: File-leg refusal stays with the recorded successor

- **GIVEN** a supersession whose file leg refused because the backup
  already named a different successor (SSW-FR-003)
- **WHEN** the SQLite store is deleted and `reIngestLearnings` runs
- **THEN** the restored target MUST keep the successor the backup
  already recorded

## MODIFIED Requirements

_None._

## REMOVED Requirements

_None._
