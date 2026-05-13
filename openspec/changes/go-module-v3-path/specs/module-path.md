## ADDED Requirements

### Requirement: Go module path includes major version suffix

The `go.mod` module declaration MUST be `module github.com/unbound-force/dewey/v3`.

#### Scenario: go install resolves to latest v3.x release

- **GIVEN** the module path in `go.mod` is `github.com/unbound-force/dewey/v3`
- **WHEN** a user runs `go install github.com/unbound-force/dewey/v3@latest`
- **THEN** Go resolves to the latest v3.x tag (e.g., v3.3.0) and installs the current binary

#### Scenario: go install with explicit version tag

- **GIVEN** the module path in `go.mod` is `github.com/unbound-force/dewey/v3`
- **WHEN** a user runs `go install github.com/unbound-force/dewey/v3@v3.2.0`
- **THEN** Go downloads and builds the v3.2.0 source without a module path mismatch error

### Requirement: All internal imports use v3 module path

Every `.go` file that imports a Dewey subpackage MUST use the `github.com/unbound-force/dewey/v3/<pkg>` import path. No references to `github.com/unbound-force/dewey/<pkg>` (without `/v3`) SHALL remain in `.go` source files.

#### Scenario: Build succeeds with rewritten imports

- **GIVEN** all import paths have been rewritten to include `/v3`
- **WHEN** `go build ./...` is run
- **THEN** the build succeeds with no import resolution errors

#### Scenario: No stale import references remain

- **GIVEN** the import rewrite is complete
- **WHEN** searching `.go` files for `"github.com/unbound-force/dewey/` (without `/v3`)
- **THEN** zero matches are found (excluding `go.sum`, spec artifacts, and historical docs)

## MODIFIED Requirements

### Requirement: go install documentation

The documented `go install` command in README.md MUST be updated from `go install github.com/unbound-force/dewey@latest` to `go install github.com/unbound-force/dewey/v3@latest`.

Previously: `go install github.com/unbound-force/dewey@latest`

#### Scenario: README install instructions match module path

- **GIVEN** the module path has been updated to include `/v3`
- **WHEN** a user follows the `go install` instructions in README.md
- **THEN** the command installs the latest v3.x release successfully

### Requirement: AGENTS.md module path reference

The module path reference in AGENTS.md MUST be updated to `github.com/unbound-force/dewey/v3`.

Previously: `github.com/unbound-force/dewey`

## REMOVED Requirements

None.
