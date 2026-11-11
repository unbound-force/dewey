# Delta Spec: curator-agent

<!--
  Requirement numbering allocation
  ================================
  Capability: curator-agent (prefix CA)
  Allocated to this change: CA-FR-013 .. CA-FR-014
  Reserved for future curator-agent changes: CA-FR-015 .. CA-FR-099
  Allocated by curator-agent-human-gates: CA-FR-001 .. CA-FR-008
    in openspec/changes/curator-agent-human-gates/specs/
    curator-agent/spec.md. CA-FR-003 states the exact Dewey MCP
    tool allowlist; this delta extends that enumeration additively.
  Allocated by curator-tool-allowlist: CA-FR-009 .. CA-FR-012
    in openspec/changes/curator-tool-allowlist/specs/
    curator-agent/spec.md. CA-FR-009 states the frontmatter-enforced
    "deny-all-then-allow-six" tools block this delta extends to
    seven; CA-FR-010..CA-FR-012 cover the cross-server deny,
    frontmatter/body parity, and the host acceptance matrix. That
    change landed on main while this change was in flight, so this
    delta takes the next free range and never renumbers it.
  Sibling capabilities:
    - knowledge-verification (prefix KV): KV-FR-001 .. KV-FR-005
      define the read-only `verify_candidates` tool this delta
      adds to the allowlist.
    - knowledge-distillation (prefix KD): KD-FR-104 .. KD-FR-107
      define the command-side verify step that calls the tool.
  The capability has no archived main spec yet, so this delta uses
  ADDED requirements. A MODIFIED operation against CA-FR-003 would
  be refused at archive time because no target spec exists in
  openspec/specs/. These requirements therefore amend CA-FR-003's
  allowlist by explicit reference rather than restating it.
  Archive-time reconciliation: if curator-agent-human-gates or
  curator-tool-allowlist archives first, CA-FR-013's additive
  extension is folded into the archived allowlist text (CA-FR-003
  and CA-FR-009 respectively), never left as a silent
  contradiction. The ids never collide: this change allocates
  CA-FR-013..CA-FR-014, clear of both siblings.
  Requirement ids MUST stay inside the allocated range.
-->

## ADDED Requirements

### Requirement: Verification Tool in the Curator Allowlist

**ID**: CA-FR-013

The curator's Dewey MCP tool allowlist, stated by CA-FR-003, MUST
additionally include exactly one tool: `verify_candidates`, and
the frontmatter-enforced shape stated by CA-FR-009 (the sibling
curator-tool-allowlist change) MUST be extended from six to seven
`true` entries by adding `dewey_verify_candidates` after the
`"dewey_*": false` deny. The allowlist text lives in two tracked
places that MUST stay byte-identical: the tracked
`.opencode/agents/curator.md` and the `agents.go` scaffold mirror
that `dewey init` writes. Both MUST list the tool in the body
allowlist and in the `tools:` map and MUST continue to forbid
every other Dewey MCP tool. No other allowlist entry MUST change:
`lint`, `list_pages`, `compile`, `curate`, `store_compiled`, and
`promote` remain allowed, and every tool outside that set remains
forbidden.
This extension carries a recorded human authorization (proposal.md,
"Recorded Human Authorizations"): agent tool-access settings are a
protected gatekeeping category, and the authorization was issued
at the 2026-11-11 spec-review pause for issue #151.

#### Scenario: Allowlist names the verification tool
- **GIVEN** the tracked `.opencode/agents/curator.md`
- **WHEN** its MCP allowlist statement is extracted
- **THEN** it MUST name `verify_candidates`
- **AND** it MUST still name exactly `lint`, `list_pages`,
  `compile`, `curate`, `store_compiled`, and `promote`

#### Scenario: Scaffold mirror stays byte-identical
- **GIVEN** the `agents.go` scaffold mirror of `curator.md`
- **WHEN** it is compared to the tracked
  `.opencode/agents/curator.md`
- **THEN** the two MUST be byte-identical
- **AND** the mirror MUST name `verify_candidates` in its
  allowlist

#### Scenario: Command stays within the allowlist
- **GIVEN** the tracked `.opencode/commands/dewey-distill.md`
- **WHEN** the Dewey MCP tools it references are inspected
- **THEN** every referenced tool MUST be within the extended
  allowlist

#### Scenario: No other entry changes
- **GIVEN** the curator agent frontmatter and body
- **WHEN** this change is reviewed
- **THEN** no existing allowlist entry MUST be removed or renamed
- **AND** no tool outside the extended set MUST become allowed

#### Scenario: Frontmatter allow set extended to seven
- **GIVEN** the tracked `.opencode/agents/curator.md` and the
  `agents.go` mirror
- **WHEN** each frontmatter `tools:` block is parsed in entry
  order
- **THEN** `"dewey_*": false` MUST precede every `dewey_*`
  `true` entry
- **AND** exactly seven `dewey_*` entries MUST carry `true`:
  `dewey_lint`, `dewey_list_pages`, `dewey_compile`,
  `dewey_curate`, `dewey_store_compiled`,
  `dewey_verify_candidates`, and `dewey_promote`
- **AND** no other `dewey_*`-matching key MUST carry `true`

### Requirement: Read-Only Boundary Preserved

**ID**: CA-FR-014

`verify_candidates` is read-only: it changes no tier, validity,
page, or file and never promotes. Granting it MUST NOT widen the
curator's mutation boundary. The curator MUST NOT gain a new way to
promote, write a page, or edit a source, test, or configuration
file.

#### Scenario: Read-only tool grants no mutation
- **GIVEN** the curator agent running under the host
- **WHEN** it calls `verify_candidates`
- **THEN** no page's tier, validity, content, or properties MUST
  change
- **AND** no file MUST be written

#### Scenario: Promotion remains human-gated
- **GIVEN** a `verify_candidates` result
- **WHEN** the curator continues the pipeline
- **THEN** `promote` MUST still require the human's review-gate
  selection
- **AND** verification MUST NOT pre-select or promote anything

#### Scenario: Permission rules unchanged
- **GIVEN** the curator agent frontmatter
- **WHEN** its `tools:` and `permission:` blocks are inspected
- **THEN** every existing tool flag and permission rule MUST be
  unchanged

## MODIFIED Requirements

None.

## REMOVED Requirements

None.
