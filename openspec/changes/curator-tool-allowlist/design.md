## Context

The curator agent shipped by `curator-agent-human-gates` (#152) declares
its Dewey MCP boundary in two places: the frontmatter `tools:` block
restricts only built-in tools (`read`/`bash` granted; `write`, `edit`,
`webfetch` denied), and the Markdown body tells the model its Dewey MCP
surface is limited to six tools. Post-merge review of #187 (issue #192)
observed that only the second is policy the model can be talked out of:
every `dewey_*` MCP tool — including `delete_page`, `rename_page`,
`store_learning`, and `bulk_update_properties` — remains callable at the
host layer, and nothing denies the second configured MCP server
(`replicator`). The prior change's design recorded this honestly (D3:
"V1 cannot scope individual MCP tools") and named V2 `permissions` rules
as the future hard-enforcement path.

What changed since then is the evidence about the `tools:` map itself:
OpenCode's V1→V2 translation (verified against the published migration
guide) maps each V1 `tools` entry to a permission rule whose `action` is
the map key with `resource: "*"`, and V2 actions support wildcards with
ordered, last-matching-rule-wins semantics. The curator file already
relies on exactly that deny-first-then-exception ordering in its nested
`bash` permission map (`"*": deny` before
`"dewey gate auto-answer": allow`). Issue #192 therefore prescribes the
same shape one level up: deny `"dewey_*": false`, re-enable the six
allowlisted tools with `true` entries.

The tracked copy is `.opencode/agents/curator.md`; `dewey init`
scaffolds it from the `deweyAgentDefinitions["curator.md"]` raw string
literal in `agents.go`; `TestInitCmd_ScaffoldsCuratorAgentFromMap`
asserts all three (tracked, map entry, scaffolded) are byte-identical.
`TestDeweyDistill_CuratorBindingAndHumanGates` already extracts the
prompt-text allowlist (`agentAllowlistEntries`) and the Dewey tool
registry (`registeredMCPToolNames`, derived from `server.go`).

This repository's `opencode.json` configures two local MCP servers:
`dewey` and `replicator`.

## Goals / Non-Goals

### Goals

- Make the curator's six-tool Dewey MCP allowlist host-enforced through
  the frontmatter `tools:` map, in the deny-all-then-allow-six shape
  issue #192 prescribes.
- Deny the curator every other configured MCP server
  (`replicator_*`), with the entry set derived from this repository's
  OpenCode configuration.
- Keep the tracked file, the embedded `agents.go` map entry, and every
  `dewey init` scaffolded copy byte-identical.
- Make the frontmatter shape, its parity with the prompt-text allowlist,
  and its coverage of the configured servers mechanically checked, so
  the lists cannot drift apart.
- Document that the glob keys carry the MCP server key, so a repository
  using a different key knows it must adjust the globs.

### Non-Goals

- Converting the agent file to the V2 `permissions` rule format. Each
  agent file is read as exactly one format — a file carrying any V2
  field (`permissions`, `system`, `disabled`, `request`) is read as V2
  and its V1 fields ignored — so this change stays in V1 like every
  sibling agent. The V2 conversion remains the repository-wide migration
  the prior change recorded.
- Changing any other agent's tool flags, the `permission:` block, the
  `/dewey-distill` command, `slash_commands.go`, or any MCP tool
  contract.
- Hard-scoping MCP tools for agents other than the curator.
- Requiring the gate-side server-key verification: it is an optional
  stretch item from #192 (see D7).

## Decisions

### D1 Stay in V1; express the boundary as `tools:` glob entries

The file keeps its V1 frontmatter (`tools:` + `permission:`) and gains
glob-keyed entries in the `tools:` map. Rationale: the issue prescribes
the `tools` map; every sibling agent in this repository is V1; and V1
frontmatter is deprecated but still translated automatically by the
host, with each file read as exactly one format. Mixing V2 fields into
this file would disable its V1 fields, breaking the existing
`permission:` carve-out. The added block (appended after `webfetch`,
denies grouped before allows):

```yaml
tools:
  read: true
  bash: true
  write: false
  edit: false
  webfetch: false
  "dewey_*": false
  "replicator_*": false
  "dewey_lint": true
  "dewey_list_pages": true
  "dewey_compile": true
  "dewey_curate": true
  "dewey_store_compiled": true
  "dewey_promote": true
```

The existing five entries are untouched. YAML requires the quoted key
form (`"dewey_*"`) because a bare `*` would otherwise parse as a YAML
alias marker; the file's `bash` map already uses quoted keys.

### D2 Deny-first ordering, mirroring the proven `bash` map

Only one ordering constraint is semantically load-bearing: the
`"dewey_*"` deny MUST precede the six `dewey_*` allow entries, so a
last-matching-rule-wins matcher resolves an allowlisted tool to `true`
and every other `dewey_*` tool to `false`. This is the same
deny-broad-then-allow-specific ordering the file's nested `bash`
permission map already relies on, and the same ordering convention the
repository's `.opencode/opencode.json` uses for its
`external_directory` map. `"replicator_*"` overlaps no other pattern, so
its position is stylistic; it is grouped with the other deny for
auditability. Ordering through a YAML map is preserved by insertion
order, the same property the `bash` map depends on.

### D3 Other-server denial is enumerated per configured server, not a blanket `*`

A blanket `"*": false` followed by re-allows would be the strongest
deny-by-default posture, but it would also deny every unlisted built-in
(`grep`, `glob`, `list`, `skill`, the question tool) that the curator
needs to function and that the human gates depend on, forcing the file
to enumerate the full built-in surface — brittle, and it risks breaking
the gates the prior change carefully kept askable. Instead, the design
adds one `<server-key>_*: false` entry per non-Dewey server configured
in this repository's `opencode.json` (currently `replicator`). The shape
test derives the configured server keys from `opencode.json` and asserts
each non-`dewey` key has a matching deny entry, so adding a server to
the repository config fails the test until the curator's deny list is
updated — the same drift protection the issue asks for, applied to the
server list. A scaffolded copy in another repository carries this
repository's entries; adjusting them is that repository's documented
responsibility (D6).

### D4 Glob semantics are verified, not assumed — with a bounded fallback

Whether the installed host honors wildcard keys in a V1 `tools:` map is
the open question issue #192 hedges ("if opencode's tool globs allow
it"). Evidence it works: the V1→V2 translation turns each `tools` key
into a permission-rule `action`, and V2 actions support wildcards. But
the Go test suite cannot run the host, so enforcement is pinned two
ways:

1. **At-rest assertions** (Go tests): the deny-all-then-allow-six shape,
   the ordering, the other-server coverage, and the parity with the
   prompt text are all asserted on the tracked file and the map entry.
2. **Runtime verification** (manual acceptance matrix): under the
   installed host, `dewey_lint` still succeeds while
   `dewey_delete_page` and a `replicator_*` call are refused by the
   frontmatter.

If the runtime verification shows the glob keys are inert, the change
MUST NOT silently accept an inert boundary. The bounded fallback is
explicit enumeration: `false` entries for every registered Dewey MCP
tool except the six (the registry is mechanically derivable —
`registeredMCPToolNames` already parses `server.go`), kept in lockstep
by a test that fails whenever a new MCP tool is registered without
updating the curator's deny list. If neither the glob form nor the
enumerated form is honored by the host, the gap is escalated per
gatekeeping integrity rather than papered over.

The failure mode of the glob form is benign while unverified: inert
entries leave the agent exactly as privileged as today (prompt-text
policy), never more.

### D5 Parity and shape are asserted with an ordered frontmatter parse

The new tests in `cli_test.go` parse the curator frontmatter with
`gopkg.in/yaml.v3` `yaml.Node` (already a production dependency,
v3.0.1), which preserves mapping key order — required for the
deny-before-allow assertion. A hand-rolled line parser in the style of
`gate.go`'s `frontmatterValue` is an acceptable alternative if the
yaml dependency is unwelcome in the test; `yaml.Node` is preferred
because it handles quoting and nesting without bespoke code. The tests:

- **Shape test** (`TestCuratorAgent_FrontmatterToolAllowlist`): split
  the frontmatter; walk the `tools:` mapping in order; assert
  `"dewey_*"` is `false` and precedes every `dewey_*`-matching `true`
  entry; assert exactly the six expected `dewey_<tool>: true` entries;
  assert no other `dewey_*`-matching key is `true`; assert the built-in
  five entries are unchanged; assert every non-`dewey` server key
  configured in `opencode.json` (parsed for its `mcp` map keys) has a
  `<key>_*: false` entry; assert the `permission:` block still carries
  its expected entries (`question: allow`; `task`, `edit`, `write`,
  `patch`, `webfetch` denied; the `bash` map with `"*": deny` before
  the single gate allow). The same assertions run against the
  `deweyAgentDefinitions["curator.md"]` map entry.
- **Parity test** (`TestCuratorAgent_AllowlistParity`): collect the
  frontmatter `dewey_*` `true` keys, strip the `dewey_` prefix, and
  assert set-equality with `agentAllowlistEntries(body)` — reusing the
  existing extractor — so the prompt-text list and the frontmatter list
  cannot drift apart in either direction.
- **Byte identity** (existing): `TestInitCmd_ScaffoldsCuratorAgentFromMap`
  already asserts tracked == map entry == scaffolded; because the change
  updates the tracked file and the map entry in lockstep, the new
  frontmatter shape is transitively asserted for every scaffolded copy.
  No new byte-identity test is needed; the existing one is the guard
  and must stay green.

The shape assertions run against the tracked copy and the map entry —
two inputs, one expected shape — rather than asserting the scaffolded
bytes separately, because byte identity makes the third copy provably
identical.

### D6 Shipping lockstep and the server-key coupling

`agents.go` mirrors the edited file byte-for-byte. The new entries
contain no backticks, so the raw string literal form is unaffected
(the map's GoDoc already forbids backticks in entries). The glob keys
carry the MCP server key `dewey` from `opencode.json`; a repository
configuring the server under a different key gets no enforcement from
these entries and MUST adjust both the `"dewey_*"` deny and the six
allow entries (plus its own other-server denies) to its configured key.
This coupling is stated in README and AGENTS.md so the adjustment is
discoverable, and the optional gate-side check (D7) would make an
unadjusted key fail loudly.

### D7 Optional stretch: gate-side server-key verification

Issue #192 suggests `dewey gate auto-answer` could verify that the
configured MCP server key matches the one the globs assume. The natural
shape: parse the project OpenCode configuration
(`pluginConfigCandidates` already enumerates them), read the `mcp` map
keys, and refuse when a server whose command invokes the `dewey` binary
is keyed as something other than `dewey` — naming the configured key
and the expected key, fail-closed like every other gate refusal. This
is recorded as an optional stretch task, not a requirement: the issue's
acceptance criteria do not include it, and the at-rest shape plus the
documented coupling deliver the enforcement the issue asks for. Deferring
it MUST be recorded in the tasks rather than silently dropped. If
implemented, it lands in `gate.go` with unit tests over fixture
configs, following the existing fail-closed matrix.

### D8 No regression

No other agent file changes. The curator's `permission:` block is
untouched — the `bash` carve-out, the question allow, and the mutation
denials all stand exactly as shipped. `dewey init` behavior is unchanged
(same never-overwrite scaffolding; only the scaffolded bytes change).
`slash_commands.go`, the `/dewey-distill` command, and every MCP tool
contract are untouched. The repository's `opencode.json` gains nothing.

## Assumptions

### A1 V1 `tools:` glob support (verified at implementation time)

The V1→V2 translation maps `tools` keys to permission-rule actions, and
V2 actions support wildcards with last-matching-rule-wins ordering.
Whether the installed host applies that to glob keys in a V1 `tools:`
map is verified by the manual acceptance matrix, with the D4 fallback
if inert. The translation behavior was verified against OpenCode's
published migration guide during this change's planning.

### A2 MCP tool flat naming (verified)

MCP tools are addressed as `<server-key>_<tool>` (for example
`dewey_lint`), confirmed by this repository's own agent documentation
(`dewey_search`, `dewey_get_page`) and OpenCode's documentation.

### A3 Each agent file is read as exactly one format (verified)

A file using any V2 field is read as V2 with its V1 fields ignored;
a file with only V1 fields is read as V1 and translated automatically.
Verified against the migration guide; this is why the change adds V1
`tools:` entries rather than V2 `permissions` rules.

### A4 Existing parity test remains the scaffold guard

`TestInitCmd_ScaffoldsCuratorAgentFromMap` and
`TestDeweyDistill_CuratorBindingAndHumanGates` already pin the
byte-identity and the prompt-text allowlist; this change extends the
surface they guard rather than replacing them.

## Risks / Trade-offs

- **Glob keys may be inert on the installed host.** The whole
  enforcement rests on the host honoring `"dewey_*"` in a V1 `tools:`
  map. Mitigation: D4's implement-time verification, the enumerated
  fallback, and the manual acceptance matrix. Failure mode while
  unverified is today's behavior, not a regression; the design does not
  claim enforcement the host has not demonstrated.
- **Map ordering may not survive translation.** The deny-before-allow
  shape assumes insertion order is preserved when the `tools:` map is
  translated to ordered rules. Mitigation: the same property is already
  load-bearing for the file's `bash` map; the acceptance matrix checks
  a non-allowlisted call is actually refused.
- **Other repositories' servers are not covered.** The deny entries
  enumerate this repository's configured servers; a scaffolded copy in
  another repository needs adjustment. Mitigation: documented coupling
  (D6), the optional gate-side check (D7), and the repo-local test that
  fails when this repository's config gains an uncovered server (D3).
- **Stale scaffolded copies.** `dewey init` never overwrites, so users
  holding the old prompt-only curator keep it until they update
  deliberately. Accepted, matching the prior change's contract.
- **Enum-lockstep cost if the fallback is needed.** The enumerated form
  couples the agent file to the MCP tool registry; every new tool
  requires a curator deny-list update. Accepted only as the fallback —
  the glob form needs no such lockstep — and the coupling is enforced
  by a failing test rather than by memory.
- **Manual host verification.** Frontmatter enforcement cannot be
  exercised by the Go suite. Accepted; enumerated in the acceptance
  matrix, the same pattern the prior change used.

## Constitution Alignment

The proposal records PASS for all four principles. This design preserves
them: no new dependency and a benign failure mode when the host lacks
glob support (I); the boundary is a declared at-rest artifact in the
agent definition, not runtime coupling (II); the frontmatter is directly
auditable and its shape, parity, and shipping identity are
machine-checked (III); the at-rest assertions are standard-library Go
tests over tracked files while host-dependent enforcement is enumerated
as manual acceptance (IV).
