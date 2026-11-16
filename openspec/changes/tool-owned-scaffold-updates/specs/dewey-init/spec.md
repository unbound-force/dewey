# Delta Spec: dewey-init

<!--
  Requirement numbering allocation
  ================================
  Capability: dewey-init (prefix DI)
  Allocated to tool-owned-scaffold-updates: DI-FR-001 .. DI-FR-002
  Reserved for future dewey-init changes: DI-FR-003 .. DI-FR-099
  Custody: dewey-init is a new consolidating capability created by
  this change; it has no archived main spec and no prior DI ids.
  Its subject matter was previously carried by three pending,
  unarchived deltas under other capability names —
  fix-init-scaffolding (init-flow), command-dir-plural
  (init-command-dir), and dewey-init-dcp-scaffold
  (init-dcp-scaffold). From this change forward, the re-init
  contract for tool-owned scaffolds lives here: DI-FR-002
  supersedes the preservation-only re-init behavior those pending
  deltas state — notably init-command-dir's
  "Re-initialization with existing commands → not overwritten"
  scenario — which is recorded as historical context inside
  DI-FR-002 rather than modified in place. Sibling capability:
  dewey-init-ownership (prefix DIO) lives in
  specs/dewey-init-ownership/ in this change. Ids MUST stay inside
  the allocated range.
-->

## ADDED Requirements

### Requirement: dewey init Scaffold Write Discipline

**ID**: DI-FR-001

All tool-owned scaffold writes — slash commands, agent
definitions, and workflow template examples — MUST share one write
discipline: map entries MUST be iterated in sorted key order; a
failed or partial create MUST leave no residue and MUST fail
`dewey init` naming the target path; a failed overwrite MUST leave
the existing file intact (atomic replace) and MUST fail init
naming the path; a target-directory creation failure MUST fail
init naming the directory.

One exception is specified for the create path. An exclusive create
that fails because the path already exists — a concurrent
`dewey init` created it between classification and the exclusive
open — MUST be treated as a benign race: the run MUST NOT fail,
MUST NOT replace the file the concurrent run wrote, and MUST log
`skip (concurrent)` at info level naming the path. Concurrent init
runs are otherwise unserialized, as today.

#### Scenario: Write failure fails init naming the path

- **GIVEN** `.opencode/` exists and `.opencode/commands/` is
  read-only so a slash-command write fails
- **WHEN** `dewey init` is run
- **THEN** init fails with an error naming the command file path
- **AND** no partial file remains in the directory

#### Scenario: Failed update leaves the existing file intact

- **GIVEN** a tool-owned file classified for update and a write
  failure during the overwrite
- **WHEN** the failure occurs
- **THEN** the pre-existing file content is unchanged on disk

#### Scenario: Concurrent create is a benign race

- **GIVEN** a tool-owned path is missing at classification and a
  concurrent `dewey init` creates it before the exclusive open
  fails with an already-exists error
- **WHEN** `dewey init` is run
- **THEN** init does not fail and does not replace the concurrent
  writer's file
- **AND** `skip (concurrent)` is logged at info level naming the
  path

### Requirement: dewey init Scaffold Refresh

**ID**: DI-FR-002

`dewey init` MUST apply the tool-owned refresh decision defined by
the `dewey-init-ownership` capability (DIO-FR-004) to every
tool-owned scaffold on every invocation — slash commands in
`.opencode/commands/`, agent definitions in `.opencode/agents/`,
and workflow template examples in `.github/workflows/` — without
requiring any flag. Re-running `dewey init` after upgrading Dewey
MUST bring untouched tool-owned scaffolds current; customized files
MUST be preserved.

The composability guards are unchanged: command and agent
scaffolding MUST NOT run, and MUST NOT create `.opencode/` or any
subdirectory, when `.opencode/` is absent. The migration of
Dewey-owned files from `.opencode/command/` (singular) to
`.opencode/commands/` MUST still run before classification; a
migrated file is then classified like any other existing file.

Previously (historical context for this consolidating capability,
not a modification baseline): `dewey init` only created missing
scaffold files. Any existing scaffold — whether an untouched copy
of an older embedded version or a user-customized file — was
unconditionally preserved, so scaffolded commands and agents never
received updates on re-init. That preservation-only contract is
superseded for tool-owned scaffolds by the refresh decision above;
the pending deltas that still state it are named in the allocation
header.

#### Scenario: Re-init after upgrade refreshes untouched commands

- **GIVEN** a vault whose `.opencode/commands/dewey-compile.md` is
  an untouched scaffold of an older Dewey version (marker digest
  still matches its content) while the current binary embeds newer
  content
- **WHEN** `dewey init` is run
- **THEN** `dewey-compile.md` is rewritten with the current
  embedded content
- **AND** missing commands are still created and customized
  commands are still preserved

#### Scenario: Re-init refreshes untouched agents and workflow
example

- **GIVEN** `.opencode/agents/curator.md` and
  `.github/workflows/dewey-distill.yml.example` are both untouched
  scaffolds of older embedded versions
- **WHEN** `dewey init` is run
- **THEN** both files are rewritten with the current embedded
  content plus fresh markers

#### Scenario: First init is unchanged apart from markers

- **GIVEN** a directory with `.opencode/` present and no prior
  Dewey scaffolds
- **WHEN** `dewey init` is run
- **THEN** all tool-owned scaffolds are created as before, each
  additionally carrying the `dewey:scaffold` marker
- **AND** `.uf/dewey/` config files, `.gitignore` handling, and
  the DCP three-way logic behave exactly as before

#### Scenario: Composability guard is unchanged

- **GIVEN** a directory without `.opencode/`
- **WHEN** `dewey init` is run
- **THEN** no `.opencode/commands/` or `.opencode/agents/`
  directory is created and no command or agent files are written
- **AND** the workflow template example is still scaffolded into
  `.github/workflows/`

#### Scenario: Old command-directory migration still precedes
classification

- **GIVEN** a vault with `.opencode/command/dewey-store.md` (old
  singular directory) and no `.opencode/commands/`
- **WHEN** `dewey init` is run
- **THEN** the file is migrated to `.opencode/commands/` first and
  then classified by the refresh decision like any other existing
  file

## MODIFIED Requirements

None.

## REMOVED Requirements

None.

## Test Strategy

Acceptance tests drive `newInitCmd()` with `--vault` over
`t.TempDir()` trees covering: refresh of untouched drifted scaffolds
per kind, preservation of customized scaffolds, marker bootstrap of
byte-identical marker-less files, first-init parity with prior
behavior, composability guards, and migration-then-classification
ordering. Write-discipline tests reuse the read-only-directory
pattern from `TestInitCmd_CuratorAgentWriteFailureFailsInit`, and
the concurrent-create EEXIST branch is exercised by invoking the
create step against an already-present path. The byte-parity tests
between embedded map values, scaffolded files, and tracked copies
are updated to compare through the marker-plus-payload
construction so the two-copy discipline is preserved. All tests run
with `-race -count=1`.
