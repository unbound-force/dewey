# Delta Spec: curator-agent

<!--
  Requirement numbering allocation
  ================================
  Capability: curator-agent (prefix CA)
  Allocated to this change: CA-FR-001 .. CA-FR-008
  Reserved for future curator-agent changes: CA-FR-009 .. CA-FR-099
  Sibling capabilities:
    - knowledge-distillation (prefix KD): this change allocates
      KD-FR-101 .. KD-FR-103 in its own delta at
      openspec/changes/curator-agent-human-gates/specs/
      knowledge-distillation/spec.md. KD-FR-001 .. KD-FR-099 are
      reserved for the pending dewey-distill change's archive-time
      numbering; no KD ids are allocated by this delta.
  Ids were consolidated during spec review, before archive, when the
  command-side requirements moved to the knowledge-distillation
  delta; no archived spec or external artifact references the
  pre-review numbering.
  Requirement ids MUST stay inside the allocated range.
-->

## ADDED Requirements

### Requirement: Curator Agent Definition

**ID**: CA-FR-001

Dewey MUST ship a `curator` agent definition at
`.opencode/agents/curator.md`. The file MUST open with YAML
frontmatter carrying a `description`, `mode: subagent`, a `tools:`
block, and a `permission:` block, followed by a Markdown body that
states the agent's purpose, its Dewey MCP tool allowlist, and its
mutation boundary.

#### Scenario: Definition parses as a subagent

- **GIVEN** the tracked file `.opencode/agents/curator.md`
- **WHEN** its YAML frontmatter is read
- **THEN** `mode` MUST equal `subagent`
- **AND** `description` MUST be a non-empty string
- **AND** a `tools:` block MUST be present
- **AND** a `permission:` block MUST be present

#### Scenario: Definition names the distillation job and boundary

- **GIVEN** the curator agent body
- **WHEN** a reviewer reads it
- **THEN** it MUST state that the agent runs knowledge curation
  through the allowlisted Dewey MCP tools
- **AND** it MUST state that direct source, test, and config edits are
  forbidden
- **AND** it MUST state that version-control operations are outside
  its authority

### Requirement: Layered Least-Privilege Capability Boundary

**ID**: CA-FR-002

The curator's capabilities MUST be denied through the layer that can
enforce them. The `tools:` block MUST grant `read` and MUST deny
`write`, `edit`, and `webfetch`. The `permission:` block MUST allow
`question` (so the mandatory human gates remain askable), MUST deny
`task`, `edit`, `write`, `patch`, and `webfetch`, and MUST deny every
shell command except the exact command `dewey gate auto-answer`, via
a nested `bash` command map whose broad deny rule precedes the
specific allow rule. The gate takes no arguments, so the map MUST NOT
carry an argument-bearing or prefix-widened allow rule: a pattern
ending in ` *` would match metacharacter-appended commands sharing
the prefix. The host matcher's semantics for the bare allow rule
MUST be verified at implementation time against the adversarial
forms enumerated in D11, each of which MUST be refused. `bash` MUST
be explicitly enabled in the `tools:` block — not omitted — because
the startup gate required by the knowledge-distillation capability
is a CLI command; its narrowing MUST live entirely in the
`permission:` block.

#### Scenario: Built-in tool flags deny mutations

- **GIVEN** the curator agent frontmatter
- **WHEN** the `tools:` block is inspected
- **THEN** `read` MUST be `true`
- **AND** `write`, `edit`, and `webfetch` MUST each be `false`
- **AND** `bash` MUST be `true`

#### Scenario: Permission block denies mutating tools

- **GIVEN** the curator agent frontmatter
- **WHEN** the `permission:` block is inspected
- **THEN** `edit`, `write`, `patch`, `webfetch`, and `task` MUST each
  be denied
- **AND** `question` MUST be allowed

#### Scenario: Bash map allows only the gate command

- **GIVEN** the curator agent's `permission:` block
- **WHEN** its `bash` map is inspected
- **THEN** a deny-all rule MUST precede a single allow rule for
  exactly `dewey gate auto-answer`
- **AND** the map MUST NOT contain an argument-bearing or
  prefix-widened allow pattern
- **AND** no other command MUST be allowed

#### Scenario: Non-gate shell commands refused at runtime

- **GIVEN** the curator agent running under the host
- **WHEN** it is asked to run `git commit`, `git push`, or any
  command other than `dewey gate auto-answer`
- **THEN** the host MUST refuse the command

#### Scenario: Subagent spawning denied

- **GIVEN** the curator agent running under the host
- **WHEN** it attempts to launch a subagent
- **THEN** the attempt MUST be refused by the `task` denial

#### Scenario: Human gates remain askable

- **GIVEN** the curator agent running under the host
- **WHEN** the distill command reaches a mandatory gate
- **THEN** the curator MUST be able to present the question to the
  human through the question tool

### Requirement: Dewey MCP Tool Allowlist

**ID**: CA-FR-003

The curator's Dewey MCP tool surface MUST be limited to exactly:
`lint`, `list_pages`, `compile`, `curate`, `store_compiled`, and
`promote`. The agent body MUST state this allowlist and MUST forbid
every other Dewey MCP tool. Persisted writes MUST flow only through
the allowlisted tools, with destinations stated per tool rather than
as one root: `compile` and `store_compiled` write under
`.uf/dewey/compiled/`; `promote` changes a page's tier in the store
database; `curate` writes under each knowledge store's configured
`path`, whose default is `.uf/dewey/knowledge/{name}` but which is
user-configurable and MAY lie outside `.uf/dewey/`. The current V1
agent format cannot hard-scope individual MCP tools, so this
boundary is enforced by the agent's stated policy, the command's
tool discipline, tracked-file assertions, and the manual acceptance
matrix; when the repository migrates the file to V2 `permissions`
rules, the allowlist MUST become a deny-by-default rule with
explicit per-tool allows.

#### Scenario: Allowlist stated exactly

- **GIVEN** the tracked `.opencode/agents/curator.md`
- **WHEN** its MCP allowlist statement is extracted
- **THEN** it MUST name exactly `lint`, `list_pages`, `compile`,
  `curate`, `store_compiled`, and `promote`
- **AND** it MUST forbid every other Dewey MCP tool

#### Scenario: Command stays within the allowlist

- **GIVEN** the tracked `.opencode/commands/dewey-distill.md`
- **WHEN** the Dewey MCP tools it references are inspected
- **THEN** every referenced tool MUST be within the allowlist

#### Scenario: Non-allowlisted mutation refused

- **GIVEN** the curator agent running under the host
- **WHEN** it is asked to call a mutating Dewey MCP tool outside the
  allowlist, such as a page or block deletion
- **THEN** the curator MUST refuse, citing its allowlist
- **AND** no such call MUST be made

#### Scenario: Persisted writes land at the tool's destination

- **GIVEN** a valid compiled or curated artifact the curator persists
  through an allowlisted tool
- **WHEN** the write lands
- **THEN** a `compile` or `store_compiled` write MUST land under
  `.uf/dewey/compiled/`
- **AND** a `curate` write MUST land under the target knowledge
  store's configured `path`
- **AND** a `promote` write MUST change the page's tier in the store
  database
- **AND** the curator MUST NOT use a direct file-write tool

### Requirement: Guard Relaxation Is User-Global Only

**ID**: CA-FR-004

Any supervisor guard relaxation for the curator MUST be granted from
the user's global supervisor configuration keyed to the `curator`
agent. The repository's committed configuration MUST NOT contain a
guard relaxation such as `blockRewardHacking: false` or
`requireStepReferenceInTools: false`, and MUST NOT contain a
curator-specific supervisor override. The permission carve-out
shipped in the agent definition is the agent's own least-privilege
boundary, not a guard relaxation. The behavior of the external
supervisor (jflowers/opencode-supervisor#48) is design rationale,
not a scenario this repository verifies.

#### Scenario: Repository config carries no relaxation

- **GIVEN** the repository after this change
- **WHEN** the committed OpenCode configuration files
  (`.opencode/opencode.json[c]` and root `opencode.json[c]`) are
  searched
- **THEN** they MUST NOT contain `blockRewardHacking` or
  `requireStepReferenceInTools` set to `false`
- **AND** they MUST NOT contain a curator-specific guard override

### Requirement: Agent Definition Shipping and Scaffolding

**ID**: CA-FR-005

`dewey init` MUST scaffold `.opencode/agents/curator.md` into a vault
that already has an `.opencode/` directory. Scaffolding MUST be
idempotent and MUST NOT overwrite an existing file. The tracked agent
file and the definition shipped by the binary MUST be byte-identical.
A scaffold write failure MUST fail `dewey init` with the target path
named, rather than reporting success with a missing agent file.

#### Scenario: Scaffolding creates the definition

- **GIVEN** a vault with an existing `.opencode/` directory and no
  `.opencode/agents/curator.md`
- **WHEN** `dewey init` runs
- **THEN** it MUST create `.opencode/agents/curator.md`
- **AND** the created bytes MUST equal the shipped definition

#### Scenario: Existing definition preserved

- **GIVEN** a vault whose `.opencode/agents/curator.md` already
  contains custom content
- **WHEN** `dewey init` runs
- **THEN** it MUST NOT overwrite the file

#### Scenario: No `.opencode/` directory

- **GIVEN** a vault with no `.opencode/` directory
- **WHEN** `dewey init` runs
- **THEN** it MUST NOT create `.opencode/agents/`
- **AND** Dewey MUST remain usable without OpenCode

#### Scenario: Tracked file matches shipped bytes

- **GIVEN** the repository's tracked agent file and the binary's
  shipped definition
- **WHEN** they are compared
- **THEN** they MUST be byte-identical

#### Scenario: Write failure surfaces

- **GIVEN** a vault whose `.opencode/agents/` directory rejects writes
- **WHEN** `dewey init` attempts to scaffold the curator
- **THEN** `dewey init` MUST fail with an error naming the target
  path
- **AND** it MUST NOT report a successful scaffold

### Requirement: Human-Gate Question Marker

**ID**: CA-FR-006

Every question that requires a human decision MUST begin its text
with the marker token `HUMAN_GATE:`. The token MUST be uppercase and
MUST be followed by a single ASCII space and the question. The marker
MUST appear at the start of the first line, with no leading
whitespace. The marker MUST NOT be used on informational or non-gate
questions. The marker grammar (uppercase token, single ASCII space,
start of first line, no leading whitespace, never on informational
lines) MUST be published to the website documentation tracking issue
unbound-force/website#371, so external auto-answer implementations
have a stable contract; whether a given plugin honors it
(jflowers/opencode-auto-question#1) is an external contract this
repository does not verify. Publishing the website pages themselves
is the website repository's obligation, not this change's.

#### Scenario: A gate question carries the marker

- **GIVEN** a mandatory scope or review gate
- **WHEN** the command asks the human to decide
- **THEN** the question text MUST begin with `HUMAN_GATE: `
- **AND** the remainder of the line MUST state the decision

#### Scenario: Marker is not a general prefix

- **GIVEN** an informational status line
- **WHEN** the command emits it
- **THEN** it MUST NOT begin with the marker token

#### Scenario: Grammar published to the tracking issue

- **GIVEN** the website documentation tracking issue
  unbound-force/website#371
- **WHEN** its body and comments are read
- **THEN** they MUST state the marker grammar (uppercase token, single
  ASCII space, start of first line, no leading whitespace, never on
  informational lines)

### Requirement: Auto-Answer Gate Tool

**ID**: CA-FR-007

Dewey MUST provide a `dewey gate auto-answer` CLI subcommand that
deterministically evaluates whether the `curator` agent is configured
for automatic question answering, prints a tool-rendered verdict, and
exits 0 on pass and 1 on refusal. The evaluation MUST read the
opencode-auto-question plugin's own configuration —
`auto-question.jsonc` in the OpenCode global config directory
(`$XDG_CONFIG_HOME/opencode` when set, else `~/.config/opencode`),
parsed as JSONC with `{env:VAR}` interpolation resolved against the
process environment — and treat its `agents` key as the authoritative
agent list, with the plugin's documented default `["autonomy"]`
applying when the key or file is absent.

Entry matching MUST be pinned so the gate and the plugin agree: every
entry is whitespace-trimmed and then case-folded before comparison,
and `{env:VAR}` interpolation applies only when the entire entry is
exactly the interpolation token — a partial interpolation inside a
larger entry is treated as unverifiable rather than matched.

The tool MUST additionally verify that the curator binding is
resolvable: `.opencode/agents/curator.md`, relative to the working
directory, MUST exist and its frontmatter MUST declare
`mode: subagent`. The tool MUST additionally scan project and global
OpenCode configurations and plugin discovery directories for plugin
registration and report the result as advisory context that does not
change the verdict.

The tool MUST refuse (exit 1) when the effective agent list contains
`curator` (after normalization), the wildcard `*`, or an entry whose
interpolation resolves to `curator`; when the curator definition is
missing or unparseable; and whenever the plugin configuration cannot
be located, verified, read, parsed, or interpreted. That fail-closed
surface includes the pre-read steps: a config directory that cannot
be resolved (both `$XDG_CONFIG_HOME` and `$HOME` unset) and a stat of
`auto-question.jsonc` that fails for any reason other than
file-not-found (for example a permission error on the config
directory). It also includes an `agents` value that is not an array
of strings, an entry whose environment variable is unset, and an
entry carrying a partial interpolation. The refusal output MUST name
the configuration file, the reason, and the corrective action.

#### Scenario: Curator configured for auto-answer

- **GIVEN** a plugin config whose `agents` list includes `curator`
- **WHEN** `dewey gate auto-answer` runs
- **THEN** it MUST exit non-zero
- **AND** its output MUST name the config file, the offending entry,
  and the fix

#### Scenario: Wildcard configured for auto-answer

- **GIVEN** a plugin config whose `agents` list is `*`
- **WHEN** `dewey gate auto-answer` runs
- **THEN** it MUST exit non-zero

#### Scenario: Plugin config absent

- **GIVEN** no `auto-question.jsonc` in the global config directory
- **WHEN** `dewey gate auto-answer` runs
- **THEN** it MUST exit zero
- **AND** its output MUST report that the documented default agent
  list applies

#### Scenario: Clean agent list

- **GIVEN** a parseable plugin config whose `agents` list contains
  neither `curator` nor `*`
- **WHEN** `dewey gate auto-answer` runs
- **THEN** it MUST exit zero
- **AND** its output MUST report the effective agent list and whether
  the default applied

#### Scenario: Unreadable plugin configuration

- **GIVEN** a plugin config that exists but cannot be read
- **WHEN** `dewey gate auto-answer` runs
- **THEN** it MUST exit non-zero rather than assume auto-answering
  is off

#### Scenario: Unparseable plugin configuration

- **GIVEN** a plugin config that exists but is not valid JSONC
- **WHEN** `dewey gate auto-answer` runs
- **THEN** it MUST exit non-zero

#### Scenario: Malformed agent list

- **GIVEN** a parseable plugin config whose `agents` value is not an
  array of strings, or whose entry references an environment variable
  that is unset
- **WHEN** `dewey gate auto-answer` runs
- **THEN** it MUST exit non-zero

#### Scenario: Padded or partially interpolated entry refused

- **GIVEN** a parseable plugin config whose `agents` entry is
  `" curator "` with surrounding whitespace, or an entry that embeds
  an interpolation token inside a larger string
- **WHEN** `dewey gate auto-answer` runs
- **THEN** the trimmed, case-folded `curator` entry MUST cause a
  non-zero exit
- **AND** the partial interpolation MUST be treated as unverifiable
  and exit non-zero

#### Scenario: Configuration location unverifiable

- **GIVEN** `$XDG_CONFIG_HOME` and `$HOME` are both unset, or the
  config directory cannot be stat'd for a reason other than
  file-not-found
- **WHEN** `dewey gate auto-answer` runs
- **THEN** it MUST exit non-zero rather than assume auto-answering
  is off

#### Scenario: Curator definition unresolvable

- **GIVEN** `.opencode/agents/curator.md` is absent or its frontmatter
  does not declare `mode: subagent`
- **WHEN** `dewey gate auto-answer` runs
- **THEN** it MUST exit non-zero, naming the unresolved binding

#### Scenario: Registration reported as context

- **GIVEN** any evaluated configuration
- **WHEN** `dewey gate auto-answer` prints its verdict
- **THEN** the output MUST report whether the plugin was found
  registered in the scanned project and global configurations
- **AND** a scanned configuration that exists but cannot be read or
  parsed MUST produce a warning line naming it, without changing the
  verdict

### Requirement: Compatibility and No Regression

**ID**: CA-FR-008

The change MUST NOT weaken any existing agent's tool flags,
permission rules, or command binding. The curator, the gate tool, and
the command MUST work without the auto-answer plugin and without the
supervisor installed, and MUST NOT make either a mandatory
dependency.

#### Scenario: Existing agents unchanged

- **GIVEN** every existing file under `.opencode/agents/`
- **WHEN** the change is reviewed
- **THEN** no existing tool flag, permission rule, or mode MUST change
- **AND** binding patterns such as `gaze.md` to `gaze-reporter` MUST
  remain intact

#### Scenario: Plugin and supervisor absent

- **GIVEN** a vault with no auto-answer plugin and no supervisor
- **WHEN** `dewey gate auto-answer` runs
- **THEN** it MUST exit zero on the documented default configuration
- **AND** `/dewey-distill` MUST run its pipeline as before

## MODIFIED Requirements

None.

## REMOVED Requirements

None.
