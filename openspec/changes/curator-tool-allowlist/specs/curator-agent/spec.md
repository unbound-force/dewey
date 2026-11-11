# Delta Spec: curator-agent

<!--
  Requirement numbering allocation
  ================================
  Capability: curator-agent (prefix CA)
  Allocated to this change: CA-FR-009 .. CA-FR-012
  Allocated by the prior change (curator-agent-human-gates):
    CA-FR-001 .. CA-FR-008
  Reserved for future curator-agent changes: CA-FR-013 .. CA-FR-099
  Sibling capabilities (e.g., knowledge-distillation, doctor) keep
  their requirements in their own deltas; capabilities with an archived
  main spec live under openspec/specs/<capability>/.
  The capability has no archived main spec yet (the
  curator-agent-human-gates change is pending archive), so this delta
  is expressed as ADDED requirements rather than MODIFIED ones.
  Requirement ids MUST stay inside the allocated range.
-->

## ADDED Requirements

### Requirement: Frontmatter-Enforced Dewey MCP Tool Allowlist

**ID**: CA-FR-009

The curator agent's frontmatter `tools:` block MUST deny every Dewey MCP
tool and re-enable exactly the six allowlisted tools: a `"dewey_*":
false` entry (the MCP server key `dewey` plus `*`) followed by `true`
entries for exactly `dewey_lint`, `dewey_list_pages`, `dewey_compile`,
`dewey_curate`, `dewey_store_compiled`, and `dewey_promote`. The broad
deny MUST precede the six allow entries. No other `dewey_*`-matching key
MUST carry `true`, and the existing built-in entries MUST NOT change.

#### Scenario: Deny-all-then-allow-six shape asserted

- **GIVEN** the tracked `.opencode/agents/curator.md`
- **WHEN** its frontmatter `tools:` block is parsed in entry order
- **THEN** a `"dewey_*": false` entry MUST be present
- **AND** it MUST appear before every `dewey_*`-matching `true` entry
- **AND** exactly six `dewey_*`-matching entries MUST carry `true`:
  `dewey_lint`, `dewey_list_pages`, `dewey_compile`, `dewey_curate`,
  `dewey_store_compiled`, and `dewey_promote`
- **AND** no other `dewey_*`-matching key MUST carry `true`

#### Scenario: Existing built-in flags unchanged

- **GIVEN** the curator agent frontmatter before and after this change
- **WHEN** the built-in `tools:` entries and the `permission:` block are
  compared
- **THEN** `read` and `bash` MUST remain `true`
- **AND** `write`, `edit`, and `webfetch` MUST remain `false`
- **AND** the `permission:` block MUST carry the same entries as before
  the change (`question` allowed; `task`, `edit`, `write`, `patch`,
  `webfetch` denied; the `bash` map with `"*": deny` preceding the
  single `dewey gate auto-answer` allow)

#### Scenario: Wildcard support verified, not assumed

- **GIVEN** the frontmatter carries the glob entries
- **WHEN** the runtime acceptance matrix runs under the installed host
- **THEN** the refusal of a non-allowlisted Dewey MCP tool MUST be
  demonstrated, not inferred from the file's shape
- **AND** if the host ignores wildcard keys, the change MUST fall back
  to explicit per-tool `false` entries covering every registered Dewey
  MCP tool except the six — kept in lockstep with the server's tool
  registry by test — or escalate the gap; it MUST NOT silently accept an
  inert boundary

#### Scenario: Non-allowlisted Dewey tool refused at runtime

- **GIVEN** the curator agent running under the installed host with the
  new frontmatter
- **WHEN** it attempts to call a non-allowlisted Dewey MCP tool, such as
  `dewey_delete_page`, `dewey_rename_page`, `dewey_store_learning`, or
  `dewey_bulk_update_properties`
- **THEN** the host MUST refuse the call
- **AND** the refusal MUST come from the frontmatter tool flags, not
  only from the agent declining per its prompt

#### Scenario: Allowlisted tool still callable at runtime

- **GIVEN** the curator agent running under the installed host with the
  new frontmatter
- **WHEN** it calls an allowlisted tool, such as `dewey_lint`
- **THEN** the call MUST succeed
- **AND** the human-gate question tool MUST remain askable so both
  mandatory gates still function

### Requirement: Other Configured MCP Servers Denied

**ID**: CA-FR-010

The curator agent's frontmatter `tools:` block MUST deny every MCP
server configured in the repository's OpenCode configuration other than
`dewey`, using one `<server-key>_*: false` entry per configured server
key (for this repository, `"replicator_*": false`).

#### Scenario: Other-server deny entry present

- **GIVEN** the repository's `opencode.json` configures a second MCP
  server under the key `replicator`
- **WHEN** the curator agent frontmatter is inspected
- **THEN** a `"replicator_*": false` entry MUST be present in the
  `tools:` block
- **AND** the curator MUST NOT be granted any tool from that server

#### Scenario: Server added without a deny entry fails the test

- **GIVEN** a new MCP server key is added to the repository's
  `opencode.json`
- **WHEN** the frontmatter shape test derives the configured server keys
  and checks the curator's deny entries
- **THEN** a missing `<server-key>_*: false` entry MUST fail the test

#### Scenario: Other-server tool refused at runtime

- **GIVEN** the curator agent running under the installed host
- **WHEN** it attempts to call a tool from the `replicator` server
- **THEN** the host MUST refuse the call

### Requirement: Allowlist Parity and Drift Protection

**ID**: CA-FR-011

The frontmatter allow set and the agent body's prompt-text allowlist
MUST name the same six tools, and a test MUST cross-check them so the
two lists cannot drift apart. The same shape MUST hold for the embedded
`deweyAgentDefinitions["curator.md"]` entry, and the tracked file, the
map entry, and the `dewey init` scaffolded copy MUST remain
byte-identical per CA-FR-005.

#### Scenario: Frontmatter and prompt text agree

- **GIVEN** the tracked `.opencode/agents/curator.md`
- **WHEN** the frontmatter `dewey_*` `true` entries and the body's
  allowlist bullets are extracted and compared as sets
- **THEN** they MUST contain exactly the same six tool names

#### Scenario: Drift in either direction is caught

- **GIVEN** a hypothetical edit that adds a seventh tool to the prompt
  text without a matching frontmatter `true` entry, or adds a frontmatter
  `true` entry without a matching prompt-text bullet
- **WHEN** the parity test runs
- **THEN** it MUST fail

#### Scenario: Shipped copies carry the same shape

- **GIVEN** the `deweyAgentDefinitions["curator.md"]` map entry and a
  curator scaffolded by `dewey init`
- **WHEN** their frontmatter is parsed
- **THEN** the deny-all-then-allow-six shape MUST hold
- **AND** the byte-identity test MUST keep the tracked file, the map
  entry, and the scaffolded copy identical

### Requirement: Server-Key Coupling Documented

**ID**: CA-FR-012

The `tools:` glob keys carry the MCP server key from the repository's
OpenCode configuration (`dewey` in `opencode.json`). The documentation
MUST state that a repository configuring the Dewey MCP server under a
different key MUST adjust the glob keys — the `"dewey_*"` deny, the six
allow entries, and the other-server denies — to match its configured
keys.

#### Scenario: Documentation states the key coupling

- **GIVEN** the README and AGENTS.md after this change
- **WHEN** a reader consults the curator agent documentation
- **THEN** it MUST state that the frontmatter tool globs assume the
  `dewey` server key from `opencode.json`
- **AND** it MUST state that a repository using a different key MUST
  adjust the globs accordingly

#### Scenario: Optional gate-side key verification

- **GIVEN** the optional stretch from issue #192 is implemented
- **WHEN** `dewey gate auto-answer` runs in a repository whose OpenCode
  configuration keys the Dewey MCP server as something other than
  `dewey`
- **THEN** the gate MUST refuse, naming the configured key and the
  expected key
- **AND** while the stretch is deferred, the deferral MUST be recorded
  in the change's tasks rather than left implicit

## MODIFIED Requirements

None.

## REMOVED Requirements

None.
