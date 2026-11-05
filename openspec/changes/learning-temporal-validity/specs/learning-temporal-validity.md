## ADDED Requirements

### Requirement: As-Of Pinning

`store_learning` MUST record `as_of` as `<repo>@<sha>`, where `repo`
is the basename of the vault's git top-level directory and `sha` is
the full `HEAD` commit, when the vault root is inside a git
repository. Resolution MUST use an injectable function with a bounded
2-second timeout. When resolution fails, `as_of` MUST be omitted and
the learning MUST still be stored.

#### Scenario: Vault inside a git repository
- **GIVEN** a vault root inside repository `dewey` at commit `abc123`
- **WHEN** a learning is stored
- **THEN** its properties and frontmatter MUST contain
  `as_of: dewey@abc123`

#### Scenario: Git unavailable
- **GIVEN** the `as_of` resolver returns an error
- **WHEN** a learning is stored
- **THEN** the learning MUST be stored without `as_of`

### Requirement: Evidence and Validity Inputs

`store_learning` MUST accept optional `evidence` (list of locator
strings) and `validity` (`current`, `stale-unverified`, `superseded`,
`historical`). `evidence` MUST hold at most 50 entries, each
non-empty after trimming and at most 512 characters; entries MUST be
trimmed and exact duplicates removed in order. Invalid input MUST
return an error result and MUST NOT write a page or file.

#### Scenario: Valid evidence and validity
- **GIVEN** `evidence: ["dewey:tools/lint.go", "dewey#147"]` and
  `validity: "current"`
- **WHEN** a learning is stored
- **THEN** properties and frontmatter MUST contain both values

#### Scenario: Invalid validity
- **GIVEN** `validity: "fresh"`
- **WHEN** a learning is stored
- **THEN** the tool MUST return an error result naming the valid
  values
- **AND** no page or file MUST be written

### Requirement: Two-Way Supersession

`store_learning` MUST accept optional `supersedes`: an identity or
`learning/<identity>` page name of an existing learning that has no
`superseded_by`. The new learning MUST record `supersedes`; the
target MUST gain `superseded_by` (the new identity) and
`validity: superseded` in the store and, when its markdown backup
exists, in its frontmatter. The target MUST NOT be deleted and its
tier MUST NOT change.

#### Scenario: Supersede an existing learning
- **GIVEN** learning `auth-20261101T000000-alice` exists
- **WHEN** a learning is stored with
  `supersedes: "auth-20261101T000000-alice"`
- **THEN** the new learning MUST record that `supersedes`
- **AND** the target's properties and frontmatter MUST contain the
  new identity as `superseded_by` and `validity: superseded`

#### Scenario: Unknown or already-superseded target
- **GIVEN** `supersedes` names no learning, or a learning that
  already has `superseded_by`
- **WHEN** a learning is stored
- **THEN** the tool MUST return an error result
- **AND** no page or file MUST be written

### Requirement: Round-Trip Through Re-Ingestion

`reIngestLearnings` MUST restore `as_of`, `evidence`, `validity`,
`supersedes`, and `superseded_by` from frontmatter into properties.
Learning files without these keys MUST re-ingest unchanged.

#### Scenario: Re-ingest a file with temporal fields
- **GIVEN** a learning file whose frontmatter carries all five keys
- **WHEN** the store is missing that learning at startup
- **THEN** the re-ingested page's properties MUST contain all five

## MODIFIED Requirements

_None._

## REMOVED Requirements

_None._
