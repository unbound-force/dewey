## MODIFIED Requirements

### Requirement: Workspace Directory Path

Dewey MUST use `.uf/dewey/` as the workspace directory for all persistent artifacts (store, config, sources, log, lock). The workspace path MUST be derived from the project root using a centralized constant.

Previously: Workspace directory was `.dewey/` hardcoded in ~146 locations.

#### Scenario: dewey init creates .uf/dewey/
- **GIVEN** a project directory with no `.uf/` directory
- **WHEN** the developer runs `dewey init`
- **THEN** `.uf/dewey/` is created with `config.yaml` and `sources.yaml` inside it

#### Scenario: dewey serve reads from .uf/dewey/
- **GIVEN** a project with `.uf/dewey/` initialized
- **WHEN** `dewey serve` starts
- **THEN** it opens `.uf/dewey/graph.db`, writes to `.uf/dewey/dewey.log`, and acquires `.uf/dewey/dewey.lock`

#### Scenario: No .dewey/ fallback
- **GIVEN** a project with only `.dewey/` (old format) and no `.uf/dewey/`
- **WHEN** `dewey serve` starts
- **THEN** it reports "not initialized" — it does NOT fall back to `.dewey/`

### Requirement: Lock File Path

The lock file MUST be named `dewey.lock` (not `.dewey.lock`) inside the `.uf/dewey/` workspace directory.

Previously: Lock file was `.dewey/.dewey.lock`.

#### Scenario: Lock file in new location
- **GIVEN** `dewey serve` is running
- **WHEN** `dewey doctor` checks for the lock
- **THEN** it looks for `.uf/dewey/dewey.lock` (not `.dewey/.dewey.lock`)

### Requirement: .gitignore Patterns

`dewey init` MUST append granular `.uf/dewey/` patterns to `.gitignore` for runtime artifacts. Configuration files (`sources.yaml`, `config.yaml`) MUST remain trackable.

Previously: Patterns used `.dewey/graph.db`, `.dewey/dewey.log`, etc.

#### Scenario: New repo .gitignore patterns
- **GIVEN** a new repo with no dewey gitignore patterns
- **WHEN** the developer runs `dewey init`
- **THEN** `.gitignore` contains `.uf/dewey/graph.db`, `.uf/dewey/dewey.log`, `.uf/dewey/dewey.lock`, and WAL/SHM patterns

## REMOVED Requirements

### Requirement: .dewey/ Directory Support

Support for the `.dewey/` workspace directory is removed. No fallback, no migration, no detection.

Reason: Ecosystem consolidation under `.uf/` namespace (org Spec 025). Users upgrading must `rm -rf .dewey/` and run `dewey init`.
