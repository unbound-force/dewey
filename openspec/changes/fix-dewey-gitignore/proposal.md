## Why

`dewey init` appends `.dewey/` to `.gitignore`, blanket-ignoring the entire directory. This prevents teams from version-controlling `sources.yaml` and `config.yaml` — shareable configuration that defines what the swarm indexes and which embedding model to use. New team members start with bare defaults instead of the team's curated source configuration.

Fixes GitHub issue #23.

## What Changes

Replace the blanket `.dewey/` gitignore pattern with granular patterns that ignore only runtime artifacts (database, log, lock, cache) while allowing configuration files to be tracked.

The change is in `dewey init`'s `.gitignore` append logic (`cli.go`), not in any scaffold template.

## Capabilities

### Modified Capabilities
- `dewey init`: Changes the `.gitignore` pattern from `.dewey/` to granular runtime-artifact patterns. Existing repos with the old pattern are not automatically migrated — a migration note is logged.

## Impact

- **cli.go**: The `.gitignore` append logic in `newInitCmd()` changes from writing `.dewey/` to writing individual runtime artifact patterns
- **Existing repos**: Repos already having `.dewey/` in their `.gitignore` continue to work (blanket ignore is a superset). Migration is opt-in — developers can manually update their `.gitignore` to the new pattern when ready.
- **New repos**: `dewey init` on a new repo will use the granular pattern, enabling `sources.yaml` and `config.yaml` to be committed immediately.

## Constitution Alignment

Assessed against the Unbound Force org constitution.

### I. Autonomous Collaboration

**Assessment**: PASS

Enabling `sources.yaml` to be version-controlled improves artifact-based collaboration — team members share source configuration through git, not manual steps.

### II. Composability First

**Assessment**: N/A

No change to Dewey's standalone functionality or dependencies.

### III. Observable Quality

**Assessment**: PASS

Making configuration trackable improves auditability — teams can see when and why source configuration changed via git history.

### IV. Testability

**Assessment**: PASS

The existing `TestInitCmd_GitignoreAppend` test covers the `.gitignore` logic. It will be updated to verify the new granular patterns.
