## ADDED Requirements

### Requirement: Learnings-only pull requests skip expensive CI

A pull request whose changes are all regular `.md` files under `.uf/dewey/learnings/` MUST skip the `build-and-test` and `MegaLinter` jobs, MUST be scanned for secrets, and the `CI OK` check MUST pass.

#### Scenario: Learnings-only PR

- **GIVEN** a pull request to `main` that only adds, edits or deletes regular `.md` files under `.uf/dewey/learnings/`
- **WHEN** CI and MegaLinter run
- **THEN** `changes` SHALL output `code=false`, `build-and-test` and `MegaLinter` SHALL be skipped, `gitleaks` SHALL run, and `CI OK` SHALL pass if `gitleaks` passes

#### Scenario: Secret in a learning

- **GIVEN** a learnings-only pull request whose commits contain a string matching a gitleaks rule
- **WHEN** CI runs
- **THEN** `gitleaks` SHALL fail and `CI OK` SHALL fail

#### Scenario: Mixed PR

- **GIVEN** a pull request that changes files under `.uf/dewey/learnings/` and at least one file elsewhere
- **WHEN** CI and MegaLinter run
- **THEN** `build-and-test` and `MegaLinter` SHALL run in full, and `CI OK` SHALL pass only if `build-and-test` passes

#### Scenario: Paths that only look like learnings

- **GIVEN** a pull request adding a symlink or submodule under `.uf/dewey/learnings/`, a path with a `.` or `..` segment, a path git quotes, or a rename from outside the directory into it
- **WHEN** the detector runs
- **THEN** it SHALL print `true`

#### Scenario: Detection fails

- **GIVEN** a pull request whose `changes` job fails or is cancelled, or whose checkout is not the PR merge commit
- **WHEN** `CI OK` runs
- **THEN** `CI OK` SHALL fail

#### Scenario: Push to main

- **GIVEN** a push to `main`
- **WHEN** CI and MegaLinter run
- **THEN** every job SHALL run as before, and `CI OK` SHALL pass only if `build-and-test` passes

## MODIFIED Requirements

### Requirement: CI push trigger

`ci.yml` MUST run on `push` to `main` only, and on `pull_request` to `main`.

#### Scenario: Push to a feature branch

- **GIVEN** a push to a branch other than `main`
- **WHEN** GitHub evaluates `ci.yml` triggers
- **THEN** no `push` run SHALL start; the branch's pull request run provides CI

### Requirement: Required status check

The required status check on `main` MUST be `CI OK` once this change is merged.
