## MODIFIED Requirements

### Requirement: dewey init .gitignore Pattern

`dewey init` MUST append granular runtime-artifact patterns to `.gitignore` instead of the blanket `.dewey/` pattern. The patterns MUST ignore only: `graph.db`, `graph.db-shm`, `graph.db-wal`, `dewey.log`, and `.dewey.lock`.

Previously: `dewey init` appended `.dewey/` which ignored all files including shareable configuration.

#### Scenario: New repo initialization
- **GIVEN** a repo with no `.dewey` patterns in `.gitignore`
- **WHEN** the developer runs `dewey init`
- **THEN** `.gitignore` contains granular patterns for runtime artifacts but NOT `.dewey/`

#### Scenario: Idempotent initialization
- **GIVEN** a repo where `dewey init` has already been run with the new patterns
- **WHEN** the developer runs `dewey init` again
- **THEN** no duplicate patterns are added

#### Scenario: Existing blanket pattern
- **GIVEN** a repo with `.dewey/` already in `.gitignore`
- **WHEN** the developer runs `dewey init`
- **THEN** the existing `.dewey/` pattern is NOT modified and an informational message is logged suggesting manual migration

### Requirement: Configuration files trackable

After `dewey init` on a new repo, `sources.yaml` and `config.yaml` MUST NOT be gitignored. They MUST be committable via `git add .dewey/sources.yaml .dewey/config.yaml`.

#### Scenario: Sources.yaml is trackable
- **GIVEN** `dewey init` was run on a new repo with the granular pattern
- **WHEN** the developer runs `git status`
- **THEN** `.dewey/sources.yaml` and `.dewey/config.yaml` appear as untracked files (not ignored)
