# Delta Spec: learning-temporal-validity

## MODIFIED Requirements

### Requirement: As-Of Pinning

`store_learning` MUST accept an optional `as_of` input. When it is
supplied, Dewey MUST trim it, MUST reject it with an error result —
writing nothing — unless it matches `owner/repo@<sha>` or `repo@<sha>`
where `<sha>` is 7 to 64 hexadecimal characters, and MUST record the
trimmed value as the learning's `as_of` without resolving git. When it
is omitted, `store_learning` MUST record `as_of` as `<repo>@<sha>`,
where `repo` is the base name of the vault's git top-level directory
and `sha` its `HEAD`, resolved with a 2-second timeout; when that
resolution fails, `as_of` MUST be omitted and the learning still
stored.

#### Scenario: Caller pins an external source

- **GIVEN** a vault inside a git repository
- **WHEN** `store_learning` is called with
  `as_of: unbound-force/dewey@3776105abcdef`
- **THEN** the page properties and file frontmatter carry
  `as_of: unbound-force/dewey@3776105abcdef`
- **AND** the vault's git HEAD is not resolved

#### Scenario: Malformed caller pin

- **WHEN** `store_learning` is called with `as_of: main`
- **THEN** the result is an error naming the expected shape
- **AND** no page and no learning file are written

#### Scenario: Vault inside a git repository (unchanged default)

- **GIVEN** the vault is a git repository at `dewey` / `abc123`
- **WHEN** `store_learning` is called without `as_of`
- **THEN** the learning carries `as_of: dewey@abc123`

#### Scenario: Git unavailable

- **GIVEN** the `as_of` resolver returns an error
- **WHEN** `store_learning` is called without `as_of`
- **THEN** the learning MUST be stored without `as_of`
