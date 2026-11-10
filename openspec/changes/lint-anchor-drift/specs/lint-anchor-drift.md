## ADDED Requirements

### Requirement: Anchor Checks

`lint` MUST check each learning whose `as_of` pins the vault's own
repository (the base name of its top-level directory, as `as_of` is
written), whose evidence has at least one `<repo>:<path>` entry for
that repository, and whose validity is not `superseded` or
`historical`. A learning that does not meet all three is not checked:
there is nothing in this repository to compare it against. For each
such evidence entry, `lint` MUST report `anchor_missing` when the
path does not exist at the `as_of` commit, and `anchor_drift` when
the path changed or was removed between the `as_of` commit and
`HEAD`. When the `as_of` commit is not in the repository, `lint` MUST
report one `anchor_missing` finding for the learning. Anchor checks
MUST use local git only and MUST be skipped when the vault is not a
git repository.

A learning whose evidence exceeds `store_learning`'s limits (more
than 50 entries, or an entry over 512 characters) MUST NOT be
checked: re-ingested learnings bypass those limits; a learning at
exactly those limits MUST be checked. Repeated locators for one path
(for example with and without a line suffix) MUST yield one anchor.
A cited path MUST be compared as a literal path, never as a pattern.
One anchor-check
pass MUST be bounded in total time as well as per git call, and MUST
stop when the request is cancelled.

Every lint result MUST report `anchor_checks` with `status`
`checked`, `skipped`, or `failed`, and a `reason` when not checked.

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

#### Scenario: Cited file removed
- **GIVEN** a learning pinned at commit A citing `repo:pkg/c.go`,
  present at A and deleted since
- **WHEN** `lint` runs
- **THEN** it MUST report `anchor_drift`

#### Scenario: Other repositories and issue numbers
- **GIVEN** evidence `other:x.go` or `repo#12`
- **WHEN** `lint` runs
- **THEN** those entries MUST NOT be checked

#### Scenario: Learning pinned to another repository
- **GIVEN** a learning with `as_of: other@B` and evidence
  `other:pkg/a.go`, in a vault whose repository is not `other`
- **WHEN** `lint` runs
- **THEN** it MUST NOT report an anchor finding for it

#### Scenario: File name with glob characters
- **GIVEN** a learning citing `repo:pkg/[ab].go`, a file unchanged
  since the pin, while `pkg/a.go` changed
- **WHEN** `lint` runs
- **THEN** it MUST NOT report `anchor_drift` for that learning

#### Scenario: Pass not run
- **GIVEN** a vault that is not a git repository, or a git failure
  during the pass
- **WHEN** `lint` runs
- **THEN** `anchor_checks.status` MUST be `skipped` or `failed`
  respectively, with a reason, and no anchor findings

### Requirement: Fix Marks Drifted Learnings

With `fix: true`, `lint` MUST set `validity: stale-unverified` on each
learning with an `anchor_drift` finding, in the store and in its
markdown backup when present, and MUST NOT change its tier or
content.

`fix` MUST NOT write the store or a backup for a learning whose
identity fails `ValidLearningIdentity` or whose properties are
unreadable. A learning marked in the store MUST count in
`fixed.anchor_drift` even when its backup cannot be updated; that
failure MUST be reported in `anchor_fix_warnings`. A pin that is not
hexadecimal MUST NOT be passed to git; such a learning MUST be
skipped.

`fix` MUST NOT overwrite a learning that became `superseded` or
`historical` after the check ran; it MUST report it in
`anchor_fix_warnings` instead. Rewrites of a learning's stored
properties by `fix` and by `store_learning`'s supersession link MUST
NOT lose each other's changes when they run concurrently.

#### Scenario: Unreadable properties
- **GIVEN** a learning whose stored properties are not valid JSON
- **WHEN** `fix` tries to mark it
- **THEN** the properties MUST be left unchanged

#### Scenario: Option-like pin
- **GIVEN** a learning with `as_of: repo@--output=x`
- **WHEN** `lint` runs
- **THEN** the learning MUST be skipped

#### Scenario: Backup cannot be updated
- **GIVEN** a drifted learning whose markdown backup has no
  frontmatter
- **WHEN** `lint` runs with `fix: true`
- **THEN** its stored validity MUST be `stale-unverified`, it MUST
  count in `fixed.anchor_drift`, the backup MUST be unchanged, and
  `anchor_fix_warnings` MUST name it

#### Scenario: Superseded during the pass
- **GIVEN** a drifted learning that is superseded before `fix`
  writes it
- **WHEN** `lint` runs with `fix: true`
- **THEN** it MUST keep `validity: superseded` and its
  `superseded_by`, and `anchor_fix_warnings` MUST name it

#### Scenario: Fix a drifted learning
- **GIVEN** a learning with an `anchor_drift` finding
- **WHEN** `lint` runs with `fix: true`
- **THEN** its properties and frontmatter MUST carry
  `validity: stale-unverified` and its tier MUST be unchanged

## MODIFIED Requirements

_None._

## REMOVED Requirements

_None._
