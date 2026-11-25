## ADDED Requirements

### Requirement: Learnings-only pull requests skip expensive CI

A pull request whose changed files are all under `.uf/dewey/learnings/` MUST skip the `build-and-test` and `MegaLinter` jobs, and every required check MUST still report as passing.

#### Scenario: Learnings-only PR

- **GIVEN** a pull request to `main` that only adds or edits files under `.uf/dewey/learnings/`
- **WHEN** CI and MegaLinter run
- **THEN** the `changes` job SHALL output `code=false`, `build-and-test` and `MegaLinter` SHALL be skipped, and the required `build-and-test` check SHALL count as passing

#### Scenario: Mixed PR

- **GIVEN** a pull request that changes files under `.uf/dewey/learnings/` and at least one file elsewhere
- **WHEN** CI and MegaLinter run
- **THEN** `build-and-test` and `MegaLinter` SHALL run in full

#### Scenario: File renamed into the learnings directory

- **GIVEN** a pull request that renames a file from outside `.uf/dewey/learnings/` into it
- **WHEN** the `changes` job runs
- **THEN** it SHALL output `code=true`

#### Scenario: File list unavailable or truncated

- **GIVEN** a pull request whose file list is empty or has more than 3000 files
- **WHEN** the `changes` job runs
- **THEN** it SHALL output `code=true`

#### Scenario: Push to main

- **GIVEN** a push to `main`
- **WHEN** CI and MegaLinter run
- **THEN** every job SHALL run as before

## MODIFIED Requirements

### Requirement: CI push trigger

`ci.yml` MUST run on `push` to `main` only, and on `pull_request` to `main`.

#### Scenario: Push to a feature branch

- **GIVEN** a push to a branch other than `main`
- **WHEN** GitHub evaluates `ci.yml` triggers
- **THEN** no `push` run SHALL start; the branch's pull request run provides CI
