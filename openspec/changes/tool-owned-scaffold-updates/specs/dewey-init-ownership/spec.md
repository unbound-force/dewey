# Delta Spec: dewey-init-ownership

<!--
  Requirement numbering allocation
  ================================
  Capability: dewey-init-ownership (prefix DIO)
  Allocated to tool-owned-scaffold-updates: DIO-FR-001 .. DIO-FR-006
  Reserved for future dewey-init-ownership changes:
  DIO-FR-007 .. DIO-FR-099
  Sibling capability: dewey-init (prefix DI) carries the scaffold
  write-discipline and refresh requirements in specs/dewey-init/
  in this change; this capability owns ownership classification,
  the scaffold marker, and user-owned preservation. Both
  capabilities are created by this change; no prior DIO ids exist.
  Ids MUST stay inside the allocated range.
-->

## ADDED Requirements

### Requirement: Scaffold Ownership Classification

**ID**: DIO-FR-001

`dewey init` MUST classify every path it scaffolds as either
**tool-owned** or **user-owned**. A path is tool-owned if and only
if it corresponds to an entry in an embedded scaffold map:
`deweySlashCommands` → `.opencode/commands/<name>`,
`deweyAgentDefinitions` → `.opencode/agents/<name>`,
`deweyWorkflowTemplates` → `.github/workflows/<name>`.
Classification MUST be by map membership, not by filename prefix;
every other init-written path is user-owned.

#### Scenario: Agent file without dewey- prefix is tool-owned

- **GIVEN** `.opencode/` exists and `.opencode/agents/curator.md`
  is absent
- **WHEN** `dewey init` is run
- **THEN** `curator.md` is scaffolded as a tool-owned file because
  `curator.md` is a `deweyAgentDefinitions` entry

#### Scenario: Non-map file with a Dewey-like name is never touched

- **GIVEN** `.opencode/commands/` contains a user-authored
  `dewey-notes.md` that is not a `deweySlashCommands` entry
- **WHEN** `dewey init` is run
- **THEN** `dewey-notes.md` is neither written, rewritten, nor
  removed

### Requirement: Scaffold Version Marker

**ID**: DIO-FR-002

Every tool-owned file written by `dewey init` MUST carry exactly
one marker line as the file's final line, containing the token
`dewey:scaffold`, the writing binary's version (informational
only), and the SHA-256 digest of the payload — the file content
excluding the marker line — as 64 lowercase hex characters. The
marker MUST use comment syntax valid for the file kind: an HTML
comment for markdown payloads, a `#` comment for YAML payloads.

#### Scenario: Newly scaffolded command carries a verifiable marker

- **GIVEN** `.opencode/` exists and
  `.opencode/commands/dewey-store.md` is absent
- **WHEN** `dewey init` is run
- **THEN** the written file ends with a `dewey:scaffold` marker
  line in HTML-comment syntax
- **AND** the SHA-256 of the file content preceding the marker line
  equals the digest recorded in the marker
- **AND** the digest is computed over the canonical payload — the
  embedded map value normalized to end with exactly one trailing
  newline

#### Scenario: Workflow template marker is a YAML comment

- **GIVEN** `.github/workflows/dewey-distill.yml.example` is absent
- **WHEN** `dewey init` is run
- **THEN** the written file ends with a `#`-comment
  `dewey:scaffold` marker line and remains parseable YAML

#### Scenario: Rewritten file carries a fresh marker

- **GIVEN** a tool-owned scaffold is rewritten because its content
  drifted from the embedded version
- **WHEN** the rewrite completes
- **THEN** the new file's marker digest matches the new payload,
  not the old one

### Requirement: Marker Digest Semantics

**ID**: DIO-FR-003

The marker digest MUST be computed over the canonical payload: the
embedded map value normalized to end with exactly one trailing
newline. Tool-owned classification MUST depend only on the digest;
the recorded version MUST NOT affect it.

#### Scenario: Version string does not affect classification

- **GIVEN** two tool-owned files with byte-identical payloads and
  valid markers recording different version strings
- **WHEN** `dewey init` is run
- **THEN** both files receive the same refresh decision

### Requirement: Tool-Owned Refresh Decision

**ID**: DIO-FR-004

On every invocation, for each tool-owned path, `dewey init` MUST
apply this table without any flag:

| Existing state | Decision |
|---|---|
| Missing | create: marker + payload; log `scaffolded` |
| Symlink (by lstat) | leave untouched; log `skip (customized)` |
| Unreadable existing path | fail init naming the path |
| Untouched (digest matches), current | no write |
| Untouched, drifted | overwrite: marker + payload; log `updated` |
| Marker-less, byte-identical | rewrite to add marker; log `updated` |
| Otherwise | leave untouched; log `skip (customized)` (info) |

The **Symlink** row: a tool-owned path that is a symlink —
determined by lstat before any read — MUST be treated as
customized. The symlink MUST NOT be replaced by a regular file and
nothing MUST be written through it to its referent; `skip
(customized)` is logged naming the path. The check is a single
lstat at classification; a path that becomes a symlink between
check and write is not followed, matching the TOCTOU posture of
promote's symlink guard (PW-FR-004 in
git-is-the-source-of-record-for-page-trust-tiers).

The **Unreadable** row: an existing tool-owned path that cannot be
read — a permission error (EACCES), a directory at the path, or a
transient I/O error — MUST fail init naming the path. A read
failure MUST NOT be classified as the customized outcome: an
environment failure must not be mislabeled as a user
customization, which would mask it from the operator.

#### Scenario: Untouched drifted scaffold is refreshed after
upgrade

- **GIVEN** `.opencode/commands/dewey-compile.md` was scaffolded by
  an older Dewey and never edited (its marker digest still matches
  its content), and the current binary embeds different content
- **WHEN** `dewey init` is run
- **THEN** the file is overwritten with the current embedded
  payload plus a fresh marker and `updated` is logged

#### Scenario: Untouched current scaffold is left alone

- **GIVEN** a tool-owned file whose marker digest matches its
  content and whose payload equals the embedded payload
- **WHEN** `dewey init` is run
- **THEN** the file is not rewritten and no `updated` or `skip
  (customized)` event is logged for it

#### Scenario: Customized scaffold is preserved

- **GIVEN** a tool-owned file whose marker digest does not match
  its content (the body was edited after scaffolding)
- **WHEN** `dewey init` is run
- **THEN** the file is not modified, moved, renamed, or deleted and
  `skip (customized)` is logged at info level naming the path

#### Scenario: Pre-marker legacy scaffold that differs is preserved

- **GIVEN** a file scaffolded by a Dewey version that did not write
  markers, whose content differs from the current embedded payload
- **WHEN** `dewey init` is run
- **THEN** the file is treated as customized: unchanged, with
  `skip (customized)` logged

#### Scenario: Pre-marker legacy scaffold that is byte-identical
gains the marker

- **GIVEN** a file with no marker whose content is byte-identical
  to the current embedded payload
- **WHEN** `dewey init` is run
- **THEN** the file is rewritten with the marker plus the same
  payload and `updated` is logged

#### Scenario: Malformed marker is treated as customized

- **GIVEN** a tool-owned path whose final line contains
  `dewey:scaffold` but the digest field is not 64 lowercase hex
  characters
- **WHEN** `dewey init` is run
- **THEN** the file is unchanged and `skip (customized)` is logged

#### Scenario: Symlinked tool-owned path is preserved

- **GIVEN** `.opencode/agents/curator.md` is a symlink to a
  canonical copy shared across repos, and the referent is a
  marker-valid drifted scaffold
- **WHEN** `dewey init` is run
- **THEN** the symlink is not replaced by a regular file and
  nothing is written through it to the referent
- **AND** `skip (customized)` is logged naming the path

#### Scenario: Unreadable existing file fails init

- **GIVEN** a tool-owned path exists but cannot be read — a
  permission error, a directory at the path, or a transient I/O
  error
- **WHEN** `dewey init` is run
- **THEN** init fails with an error naming the path
- **AND** the file is not modified and no `skip (customized)`
  event is logged for it

### Requirement: User-Owned File Preservation

**ID**: DIO-FR-005

The user-owned vault configuration files — `.uf/dewey/config.yaml`,
`.uf/dewey/sources.yaml`, and `.uf/dewey/knowledge-stores.yaml` —
MUST remain write-once: created only when absent, never
overwritten on re-init.

`.gitignore` is user-owned but not write-once: it MUST retain its
existing append-if-missing-patterns semantics. Dewey
runtime-artifact patterns (`.uf/dewey/graph.db` and its
`-shm`/`-wal` sidecars, `dewey.log`, `dewey.lock`) are appended
only when the file contains no Dewey pattern; the file is never
rewritten, truncated, or reordered.

`.opencode/dcp.jsonc` MUST retain its existing three-way logic,
unchanged:

1. skip when either `.opencode/dcp.jsonc` or `.opencode/dcp.json`
   contains `protectTags`
2. warn (without overwriting) when either file exists without
   `protectTags`
3. scaffold `.opencode/dcp.jsonc` when neither file exists

#### Scenario: Re-init preserves edited vault config

- **GIVEN** `.uf/dewey/` exists and the user has edited
  `.uf/dewey/config.yaml`
- **WHEN** `dewey init` is run
- **THEN** `config.yaml` is byte-for-byte unchanged

#### Scenario: DCP three-way logic is unchanged

- **GIVEN** `.opencode/` exists and `.opencode/dcp.jsonc` contains
  `protectTags`
- **WHEN** `dewey init` is run
- **THEN** the file is not modified

#### Scenario: Gitignore handling stays additive

- **GIVEN** `.gitignore` exists with user patterns and no Dewey
  runtime-artifact patterns
- **WHEN** `dewey init` is run
- **THEN** the Dewey runtime-artifact patterns are appended after
  the existing content
- **AND** the pre-existing content is otherwise byte-for-byte
  unchanged

### Requirement: Observable Scaffold Outcomes

**ID**: DIO-FR-006

`dewey init` MUST emit a structured log event at info level for
each tool-owned scaffold outcome, carrying the outcome and the
target path:

- `scaffolded` — the file was newly created
- `updated` — a tool-owned file was overwritten with the current
  embedded payload
- `skip (customized)` — a diverged, symlinked, or otherwise
  preserved file was deliberately left untouched

A create that loses a race with a concurrent `dewey init` (the
EEXIST case specified by DI-FR-001) MUST log `skip (concurrent)`
naming the path.

Iteration over scaffold map entries MUST proceed in sorted key
order so that outcomes, and any failure the run reports, are
deterministic across runs.

#### Scenario: Outcomes are logged with paths

- **GIVEN** a vault where one tool-owned command is missing, one is
  drifted-but-untouched, and one is customized
- **WHEN** `dewey init` is run
- **THEN** exactly one `scaffolded`, one `updated`, and one `skip
  (customized)` event is logged, each naming its path

#### Scenario: Deterministic iteration order

- **GIVEN** the same vault state across two `dewey init` runs
- **WHEN** both runs complete
- **THEN** the scaffold outcomes are produced in the same order and
  any write failure names the same first failing path

## MODIFIED Requirements

None.

## REMOVED Requirements

None.

## Test Strategy

The refresh decision is a pure function over (existing file
content, embedded payload) and is tested table-driven without I/O.
Marker round-trip, per-kind comment syntax, every decision-table
row, and YAML parseability of the marked workflow template are
covered in unit tests using `t.TempDir()` and `newInitCmd()` with
`--vault`, following the existing `TestInitCmd_*` patterns in
`cli_test.go`. Writer tests cover the input-acquisition edges: an
unreadable existing path fails init naming the path (never
classified as customized), a symlinked tool-owned path is preserved
without replacement or write-through, and the EEXIST
concurrent-create branch logs `skip (concurrent)` without failing.
Outcome observability is asserted by capturing log output — exactly
one `scaffolded`, one `updated`, and one `skip (customized)` event,
each naming its path — deterministic ordering is asserted across
two runs, and non-map preservation is asserted with a user-authored
Dewey-like file. Existing skip-tests (which write marker-less
custom content) must pass unmodified. All tests run with `-race
-count=1`; no test requires a running Dewey MCP server.
