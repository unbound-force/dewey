# Delta Spec: knowledge-distillation

<!--
  Requirement numbering allocation
  ================================
  Capability: knowledge-distillation (prefix KD)
  Allocated to this change: KD-FR-101 .. KD-FR-103
  Reserved for future knowledge-distillation changes from this
  repository's distill work: KD-FR-104 .. KD-FR-199
  Reserved for the pending dewey-distill change's archive-time
  numbering: KD-FR-001 .. KD-FR-099 — that change's requirements
    currently carry names, not ids, in
    openspec/changes/dewey-distill/specs/knowledge-distillation/
    spec.md ("Gated Distill Workflow" and siblings); this change
    starts at 101 so whichever change archives first cannot collide.
  Sibling capabilities:
    - curator-agent (prefix CA): CA-FR-001 .. CA-FR-008 live in
      openspec/changes/curator-agent-human-gates/specs/curator-agent/
      spec.md and own the agent-side mechanisms (permission model,
      MCP allowlist, marker grammar, gate tool) this delta's
      command-side requirements reference by id.
  The capability's main spec is not archived yet: its existing
  pipeline contract is the named requirement "Gated Distill Workflow"
  in the dewey-distill change. Because no target spec exists in
  openspec/specs/, this delta MUST use ADDED requirements — a
  MODIFIED operation would be refused at archive time. These
  requirements add the curator-binding contract alongside the
  existing gated-workflow requirement rather than restating it.
  Requirement ids MUST stay inside the allocated range.
-->

## ADDED Requirements

### Requirement: Curator-Bound Distill Command

**ID**: KD-FR-101

The `/dewey-distill` command MUST declare `agent: curator` in its
YAML frontmatter, alongside the pipeline contract already stated by
the "Gated Distill Workflow" requirement. Before any pipeline stage
it MUST run the auto-answer gate tool required by CA-FR-007
(`dewey gate auto-answer` — the only shell command the command
invokes) and MUST refuse to proceed on any verdict other than a
pass: it stops before the lint stage, relays the tool's output
verbatim, and runs no stage. Because the gate tool verifies the
curator definition resolves, a host that cannot honor the `agent:`
binding is refused rather than silently running under the session
agent. A `dewey` binary missing from `PATH`, a shell error, or a
stale binary that produces no verdict is treated exactly like a
refusal: no stage runs.

This shell invocation is the single documented exception to the
pending "Gated Distill Workflow" requirement's instruction that the
command use MCP tools rather than CLI subcommands. The startup gate
never touches the store lock, so the lock rationale for that clause
is preserved; the two requirements coexist with the carve-out stated
here and in the command file.

#### Scenario: Binding declared in frontmatter

- **GIVEN** the tracked command file
- **WHEN** its frontmatter is read
- **THEN** `agent` MUST equal `curator`

#### Scenario: Startup gate invoked and refusal relayed

- **GIVEN** the tracked command file
- **WHEN** its startup-gate section is read
- **THEN** it MUST instruct running `dewey gate auto-answer` before
  any pipeline stage
- **AND** it MUST instruct relaying the tool's output verbatim and
  running no stage on a non-pass verdict

#### Scenario: Gate passes

- **GIVEN** `dewey gate auto-answer` exits zero
- **WHEN** the command starts
- **THEN** it MUST proceed to the lint stage

#### Scenario: Gate refuses

- **GIVEN** `dewey gate auto-answer` exits non-zero
- **WHEN** the command starts
- **THEN** it MUST stop before any pipeline stage
- **AND** MUST relay the tool's refusal output verbatim without
  retrying

#### Scenario: Curator binding unresolvable

- **GIVEN** a host that cannot resolve the `agent: curator` binding
  (the definition is absent or unparseable)
- **WHEN** the command starts
- **THEN** `dewey gate auto-answer` MUST exit non-zero
- **AND** the command MUST stop before any pipeline stage

#### Scenario: Gate invocation failure

- **GIVEN** `dewey` is missing from `PATH` or the gate invocation
  fails without producing a verdict
- **WHEN** the command starts
- **THEN** it MUST stop before any pipeline stage and report the
  invocation failure

### Requirement: Distill Command Scaffold Parity

**ID**: KD-FR-102

The tracked `/dewey-distill` command file and the content shipped in
`deweySlashCommands` MUST stay byte-identical, and both MUST carry
the `agent: curator` frontmatter.

#### Scenario: Scaffold parity

- **GIVEN** the tracked command file and the `deweySlashCommands`
  entry
- **WHEN** both are compared
- **THEN** they MUST be byte-identical
- **AND** both MUST carry the `agent: curator` frontmatter

### Requirement: Human-Gated Distill Questions

**ID**: KD-FR-103

Both mandatory gate questions MUST begin with the `HUMAN_GATE:`
marker token defined by CA-FR-006. The command MUST NOT call
`compile` or `curate` before a human answers the scope question, and
MUST NOT call `promote` before a human answers the review question.
Each mandatory gate section of the command MUST carry exactly one
marker-prefixed question line, and no line outside the gate sections
MUST begin with the marker; constraint prose MUST NOT begin a line
with the literal token, so the structural assertion cannot be tripped
by documentation mentioning it and a future additional gate can add
its own section without breaking a global count.

#### Scenario: Marked scope gate

- **GIVEN** the command reaches the scope gate
- **WHEN** it asks the human which tags to compile and stores to
  curate
- **THEN** the question MUST begin with `HUMAN_GATE:`
- **AND** `compile` and `curate` MUST NOT run before a human answer

#### Scenario: Marked review gate

- **GIVEN** the command reaches the review gate
- **WHEN** it asks the human which pages to promote
- **THEN** the question MUST begin with `HUMAN_GATE:`
- **AND** `promote` MUST NOT run before a human answer

#### Scenario: Marker structure is exclusive to gate sections

- **GIVEN** the tracked command file
- **WHEN** its lines are inspected by section
- **THEN** each mandatory gate section MUST contain exactly one line
  beginning with the marker
- **AND** no line in any other section MUST begin with the marker

#### Scenario: Unselected page not promoted

- **GIVEN** a human selects a subset of the eligible draft pages
- **WHEN** the command calls `promote`
- **THEN** only the selected pages MUST be promoted

## MODIFIED Requirements

None.

## REMOVED Requirements

None.
