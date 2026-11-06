## ADDED Requirements

### Requirement: Anchor Checks

`lint` MUST check each learning that has `as_of` and whose validity
is not `superseded` or `historical`. For each `<repo>:<path>`
evidence entry whose repo equals the `as_of` repo, `lint` MUST report
`anchor_missing` when the path does not exist at the `as_of` commit,
and `anchor_drift` when the path changed between the `as_of` commit
and `HEAD`. When the `as_of` commit is not in the repository, `lint`
MUST report one `anchor_missing` finding for the learning. Anchor
checks MUST use local git only and MUST be skipped when the vault is
not a git repository.

#### Scenario: Cited file changed
- **GIVEN** a learning with `as_of: repo@A` and evidence
  `repo:pkg/a.go`, and `pkg/a.go` changed after commit A
- **WHEN** `lint` runs
- **THEN** it MUST report `anchor_drift` for that learning and anchor

#### Scenario: Cited file unchanged
- **GIVEN** a learning pinned at commit A whose cited file has not
  changed since A
- **WHEN** `lint` runs
- **THEN** it MUST NOT report an anchor finding for it

#### Scenario: Cited file never existed
- **GIVEN** a learning citing `repo:nope.go`, absent at its pin
- **WHEN** `lint` runs
- **THEN** it MUST report `anchor_missing`

#### Scenario: Other repositories and issue numbers
- **GIVEN** evidence `other:x.go` or `repo#12`
- **WHEN** `lint` runs
- **THEN** those entries MUST NOT be checked

### Requirement: Fix Marks Drifted Learnings

With `fix: true`, `lint` MUST set `validity: stale-unverified` on each
learning with an `anchor_drift` finding, in the store and in its
markdown backup when present, and MUST NOT change its tier or
content.

`fix` MUST NOT write the store or a backup for a learning whose
identity fails `ValidLearningIdentity` or whose properties are
unreadable. A pin that is not hexadecimal MUST NOT be passed to git;
such a learning MUST be skipped.

#### Scenario: Unreadable properties
- **GIVEN** a learning whose stored properties are not valid JSON
- **WHEN** `fix` tries to mark it
- **THEN** the properties MUST be left unchanged

#### Scenario: Option-like pin
- **GIVEN** a learning with `as_of: repo@--output=x`
- **WHEN** `lint` runs
- **THEN** the learning MUST be skipped

#### Scenario: Fix a drifted learning
- **GIVEN** a learning with an `anchor_drift` finding
- **WHEN** `lint` runs with `fix: true`
- **THEN** its properties and frontmatter MUST carry
  `validity: stale-unverified` and its tier MUST be unchanged

## MODIFIED Requirements

_None._

## REMOVED Requirements

_None._
