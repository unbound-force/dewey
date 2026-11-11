## Why

The curator agent (`curator-agent-human-gates`, #152) is described as
least-privilege, but its Dewey MCP tool boundary is enforced only by the
agent's own prompt text. `.opencode/agents/curator.md` tells the model its
Dewey MCP surface is limited to exactly six tools — `lint`, `list_pages`,
`compile`, `curate`, `store_compiled`, and `promote` — yet the frontmatter
`tools:` block restricts only the built-in tools (`write`, `edit`,
`webfetch` denied; `read`, `bash` granted). Nothing in the frontmatter
restricts MCP tools, so every `dewey_*` tool stays callable at the host
layer, including `delete_page`, `rename_page`, `store_learning`, and
`bulk_update_properties`. A prompt is policy the model can be talked out
of; frontmatter tool flags are host-enforced. Found in post-merge review
of #187 (which closed #152); part of #153 (umbrella). Recorded as
originating issue #192.

## What Changes

- **Frontmatter-enforced Dewey MCP allowlist**: the curator agent's
  frontmatter `tools:` map gains a deny-all-then-allow-six shape —
  `"dewey_*": false` denies every tool from the `dewey` MCP server, then
  six `true` entries re-enable exactly the allowlisted tools under their
  server-key-prefixed flat names (`dewey_lint`, `dewey_list_pages`,
  `dewey_compile`, `dewey_curate`, `dewey_store_compiled`, and
  `dewey_promote`). The broad deny precedes the specific allows, the same
  deny-first ordering the file's nested `bash` permission map already
  uses. The existing entries (`read`, `bash`, `write`, `edit`,
  `webfetch`) do not change.
- **Deny-by-default for other MCP servers**: the `tools:` map also gains
  a deny entry for every other MCP server configured in this repository's
  `opencode.json` (currently `"replicator_*": false`), so the curator
  cannot reach a second server's tools. Tool globs carry the MCP server
  key, so a repository configuring the server under a different key must
  adjust the globs; the documentation states this coupling.
- **Shipping parity**: the same change ships in the embedded
  `deweyAgentDefinitions["curator.md"]` copy in `agents.go` that
  `dewey init` scaffolds, keeping the tracked file, the map entry, and
  every scaffolded copy byte-identical.
- **Mechanical verification**: `cli_test.go` gains (a) a test that parses
  the curator frontmatter and asserts the deny-all-then-allow-six shape —
  the broad deny present and preceding the six allows, exactly six
  `dewey_*`-matching `true` entries, the other-server deny present, and
  the existing built-in flags unchanged; (b) a drift check that
  cross-checks the frontmatter allow set against the prompt-text
  allowlist under the agent's "## Dewey MCP tool allowlist" heading, so
  the two lists cannot diverge; and (c) continued reliance on the
  existing byte-identity test (`TestInitCmd_ScaffoldsCuratorAgentFromMap`),
  which now covers the new frontmatter shape transitively because the
  three copies stay byte-identical.
- **Runtime verification**: the manual acceptance matrix gains checks
  that, under the installed host, an allowlisted call (`dewey_lint`)
  still succeeds while a non-allowlisted Dewey call (`dewey_delete_page`)
  and a `replicator_*` call are refused by the frontmatter — not merely
  by prompt discipline.
- **Stretch (optional, per #192)**: `dewey gate auto-answer` may
  additionally verify that the MCP server key configured in
  `opencode.json` is `dewey` (the key the globs assume), refusing on a
  mismatch. Treated as optional; deferring it is recorded rather than
  silently dropped.

## Capabilities

### New Capabilities

- None.

### Modified Capabilities

- `curator-agent`: the capability's MCP tool allowlist moves from
  prompt-only policy to frontmatter-enforced tool flags (deny-all-then-
  allow-six plus other-server denies), with parity and drift protection
  between the frontmatter list and the prompt-text list. The capability
  has no archived main spec yet (the `curator-agent-human-gates` change
  is pending archive), so the delta is expressed as ADDED requirements
  CA-FR-009 through CA-FR-012 in the range the prior change reserved for
  future curator-agent changes.

### Removed Capabilities

- None.

## Impact

- `.opencode/agents/curator.md` — frontmatter `tools:` map gains the
  deny/allow entries; body unchanged.
- `agents.go` — the `deweyAgentDefinitions["curator.md"]` raw string
  literal mirrors the tracked file byte-for-byte (no backticks in the new
  entries, so the raw literal form is unaffected).
- `cli_test.go` — new frontmatter-shape and drift tests alongside the
  existing curator tests; existing helpers (`splitFrontmatter`,
  `agentAllowlistEntries`, `sameStringSet`) are reused.
- `AGENTS.md` / `README.md` — document the frontmatter enforcement and
  the server-key coupling of the globs; add a Recent Changes bullet.
- Website tracking — the prior change's open documentation issues
  (unbound-force/website#371 docs, #376 tutorial, #377 blog) describe the
  curator's permission model; this change extends them (or files a
  `docs:` sync issue if they are closed) to state the allowlist is now
  frontmatter-enforced.
- No Go production behavior changes beyond the `agents.go` string
  content: no MCP tool contracts, store schema, provider configuration,
  CLI surface, or module dependencies change. `gate.go` is untouched
  unless the optional stretch is implemented.

**Out of scope:** converting the agent file to the V2 `permissions` rule
format (recorded by the prior change as the repository-wide migration
path; each agent file is read as exactly one format, so this change stays
in V1), hard-scoping tools on any other agent, and weakening or removing
any existing agent's tool flags or permission rules.

## Constitution Alignment

Assessed against the Dewey constitution (v1.4.0).

### I. Composability First

**Assessment**: PASS

The change adds no dependency and no new artifact. Dewey without
OpenCode is unaffected: the frontmatter entries live in a scaffolded
agent file, and a host whose `tools` map ignores glob keys degrades to
exactly today's behavior (prompt-text policy) rather than breaking. The
scaffolding still only writes when `.opencode/` exists and never
overwrites.

### II. Autonomous Collaboration

**Assessment**: PASS

The enforcement is a declared, at-rest artifact — frontmatter fields in
a tracked file — not runtime coupling to another tool. The boundary is
communicated through the same agent-definition channel the host already
consumes; no Dewey code inspects or depends on host internals.

### III. Observable Quality

**Assessment**: PASS

The allowlist becomes auditable in the frontmatter itself: a reviewer
reads the deny and allow entries directly, and tests mechanically assert
the shape and the frontmatter-to-prompt parity. The byte-identity test
keeps the tracked file, the shipped map entry, and scaffolded copies
provably identical, so what is audited is what ships.

### IV. Testability

**Assessment**: PASS

The frontmatter shape, the drift cross-check, and byte identity are all
asserted by standard-library Go tests over tracked files — no external
services. Host-side enforcement (whether the installed host honors the
glob keys) cannot be exercised by the Go suite, so it is enumerated as
explicit manual acceptance checks in `tasks.md`, the same pattern the
prior change used for its bash-map matcher semantics.
