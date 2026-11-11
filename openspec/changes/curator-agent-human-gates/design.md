## Context

`/dewey-distill` (#145) runs lint → inventory → scope gate →
compile/curate → review gate → batch promote → report through the
running Dewey MCP server. It is tracked at
`.opencode/commands/dewey-distill.md` and is also shipped from the
`deweySlashCommands` map in `slash_commands.go`; `cli_test.go`
asserts the tracked file and the map entry are byte-identical.

The command runs under whatever agent the session started with. At
corpus scale that run quotes `--no-verify` and names hundreds of
paths, which trips the supervisor's reward-hacking and path-trace
guards. Committing `blockRewardHacking: false` and
`requireStepReferenceInTools: false` to project config silences the
guards for every agent in the repository. The command also cannot
currently prevent an automatic question-answering plugin from
answering its two mandatory gates.

Existing patterns to follow:

- `.opencode/agents/gaze-reporter.md` — V1 frontmatter with
  `description`, `mode: subagent`, and a boolean `tools:` block.
- `.opencode/agents/divisor-guard.md` — V1 `permission:` deny block
  (`edit`, `bash`, `webfetch`).
- `.opencode/commands/gaze.md` and
  `.opencode/commands/uf.constitution-check.md` — `agent:` in
  command frontmatter binds the command to an agent.

There is no agent-scaffolding mechanism today: `dewey init`
scaffolds slash commands and `dcp.jsonc` only.

Verified external contracts (checked against the opencode-auto-
question README and the OpenCode documentation during spec review;
see A1 and A2):

- The auto-question plugin is registered through the `plugin`/`plugins`
  array in `opencode.json[c]` or auto-discovered from
  `.opencode/plugins/` and `~/.config/opencode/plugins/`.
- Its agent list is NOT in the OpenCode config. The plugin reads its
  own config file at the OpenCode global config dir
  (`auto-question.jsonc`, JSONC with `{env:VAR}` interpolation) and
  auto-answers only for session agents in its `agents` key, whose
  documented default is `["autonomy"]`.
- The V1 permission model supports nested per-command `bash` maps
  (`permission.bash: {"git push *": "ask"}`) and denies the
  `question` tool for every custom agent unless
  `permission: question: allow` is set.

## Goals / Non-Goals

### Goals
- Give knowledge curation its own least-privilege agent identity.
- Bind `/dewey-distill` to that agent so normal sessions never run
  under it.
- Make the two mandatory gates human-only: a marked question plus a
  deterministic, tested startup gate that refuses when
  auto-answering is configured for the curator (or cannot be
  verified).
- Ship the agent from the binary and keep the tracked file in parity.
- Grant any guard relaxation from global user config, never the repo.

### Non-Goals
- Implementing marker handling in the auto-answer plugin
  (jflowers/opencode-auto-question#1).
- Implementing per-agent supervisor guard overrides
  (jflowers/opencode-supervisor#48).
- Changing the Dewey MCP tool contracts, store schema, or provider
  configuration.
- Weakening any existing agent's tool access or command binding.
- Hard-scoping individual MCP tools via V2 `permissions` rules (the
  repository's agent files are V1 format; see D3 for the recorded
  migration path).

## Decisions

### D1 Agent identity

Create `.opencode/agents/curator.md` in the repository's V1 agent
format: `description`, `mode: subagent`, a `tools:` block, a
`permission:` block, and a body that names the job, the MCP tool
allowlist, and the mutation boundary. The body is operational policy,
not a capability grant; the `tools:` and `permission:` blocks are the
enforceable part. The description states that the agent runs
knowledge curation through Dewey MCP tools.

The file stays in the V1 format (`tools:` + `permission:`) because
every sibling agent in this repository is V1 and the installed host
stack is V1-era. OpenCode reads each agent file as exactly one
format — a file carrying V2 fields (`permissions`, `system`,
`disabled`, `request`) is read as V2 and its V1 fields ignored — so
mixing formats in one file is not an option. See D3 for the V2
conversion note.

### D2 Layered capability denial, with one auditable carve-out

OpenCode's V1 `tools:` block offers all-or-nothing booleans per
built-in tool, and a `tools:` block disables only the tools it lists.
The V1 `permission:` block adds per-tool and per-command rules
(including nested `bash` command maps). Neither layer scopes MCP
server tools. The design therefore denies each capability through the
layer that can actually enforce it, and verifies the result under the
host:

1. `tools:` block — `read: true`; `write: false`; `edit: false`;
   `webfetch: false`. `bash` is deliberately set to `true` here
   rather than `false`: the startup gate (D8) is a CLI command, and a
   boolean `bash: false` would disable the tool entirely, making the
   gate unrunnable. Setting it explicitly to `true` (rather than
   omitting it and relying on the unlisted-tool default) keeps the
   file auditable.
2. `permission:` block — deny every capability the curator must not
   have, then carve out exactly one shell command:
   - `question: allow` — the V1 base policy denies `question` for
     every custom agent; without this override the curator could not
     ask the two human gates at all.
   - `task: deny` — the curator must not spawn subagents that could
     inherit broader capabilities.
   - `edit: deny`, `write: deny`, `patch: deny`, `webfetch: deny` —
     defense in depth beneath the `tools:` booleans.
   - `bash` as a nested command map: broad deny first, specific
     allow last (last matching rule wins, the same ordering the
     repository's `.opencode/opencode.json` uses for the
     `agent.autonomy.permission.external_directory` map, whose
     `"*": "deny"` entry precedes its specific allows):

     ```yaml
     bash:
       "*": "deny"
       "dewey gate auto-answer": "allow"
     ```

   The gate takes no arguments, so the map carries exactly one allow
   rule and no argument-bearing or prefix-widened pattern. An allow
   rule ending in ` *` is deliberately absent: if the host matches a
   nested bash pattern against the raw command string (the shape of
   the `permission.bash` example above), a trailing wildcard would
   match shell-metacharacter-appended forms such as
   `dewey gate auto-answer && <cmd>` or
   `dewey gate auto-answer; <cmd>`, widening the shell surface to
   arbitrary commands.

   Matcher semantics are pinned as an implement-time verification,
   not an assumption: the acceptance matrix (D11) exercises the
   adversarial forms above and requires each to be refused. One
   residual vector remains and is documented here rather than
   claimed closed — if the host parses argv and strips leading
   environment assignments, an invocation with a leading
   `XDG_CONFIG_HOME` assignment would match the allow rule while
   redirecting the gate's config lookup at an attacker-chosen
   directory, producing a false PASS. The matrix records this form;
   the mitigation is the supervisor's shell guards plus human review
   of the run.

The single-command carve-out is the minimal shell surface the
consented gate tool requires, and it is auditable at rest: the
tracked agent file names exactly one allowed command. The manual
acceptance matrix (D11) verifies under the installed host that
`git commit`, `git push`, and an arbitrary command are all refused
while the bare `dewey gate auto-answer` runs.

No path-scoped write permission is assumed: no verified V1 schema
exists for scoping `write`/`edit` to `.uf/dewey/**`, and the design
keeps direct writes denied outright instead.

### D3 Mutation boundary and the Dewey MCP tool allowlist

No `bash` beyond the gate command means no `git commit`, `git push`,
`git add`, or arbitrary shell. No `write`/`edit`/`patch` means no
source, test, documentation, or config change. No `task` means no
delegation to a more permissive agent.

The curator's persisted writes flow through Dewey MCP tools, and the
V1 agent format cannot deny individual MCP tools on a server the
agent also needs to read from. The design therefore fixes an explicit
allowlist — exactly the tools the distill pipeline calls:

- `lint`
- `list_pages`
- `compile`
- `curate`
- `store_compiled`
- `promote`

The agent body MUST state this allowlist and MUST forbid every other
Dewey MCP tool (page and block mutation, deletion, renaming, moving,
property updates, link writes, learning stores, decisions, journals,
flashcards, whiteboards). The `/dewey-distill` command references
only allowlisted tools, and a tracked-file test asserts both files
stay within the allowlist, so the boundary is mechanically checkable
at rest even though the host cannot hard-enforce it per tool. The
manual acceptance matrix verifies a requested non-allowlisted
mutation is refused by the curator under the host.

Server-side confinement backs the write claim: the allowlisted
tools' writes land only in the configured knowledge directories
(`compile`/`store_compiled` under `.uf/dewey/compiled/`, `curate`
under the store's configured path, `promote` as a tier change in the
store, per the knowledge-distillation capability's contracts).

V2 conversion note: when this repository migrates its agent files to
the V2 format, the curator MUST convert to the ordered `permissions`
rule list with a deny-by-default rule and explicit allows —
including `<server>_<tool>` actions for the six allowlisted MCP
tools, which is the host mechanism that hard-scopes MCP access.
Until then the allowlist is policy plus verification, documented
honestly rather than overclaimed.

### D4 Guard relaxation lives in global config

The repository gains no supervisor override. A user who needs the
curator's guards narrowed configures a per-agent override in their
global supervisor configuration, keyed to `curator`. That keeps a
relaxation scoped to one identity and out of any repository a
teammate may copy. The guard-side feature is external
(jflowers/opencode-supervisor#48) and is recorded here as design
rationale, not as a scenario this repository can verify.

The bash carve-out in D2 is not a guard relaxation: it is the
agent's own least-privilege boundary, shipped in the agent
definition, and it narrows rather than widens what any agent may do.

### D5 Command binding

Add `agent: curator` to the YAML frontmatter of
`.opencode/commands/dewey-distill.md`, matching `gaze.md` and
`uf.constitution-check.md`. The issue's acceptance criterion says a
command `agent:` switches the agent even when the session is on
another agent; that is host behavior, verified manually (see A3),
not asserted by Go tests.

The knowledge-distillation delta adds the command-side requirements
(KD-FR-101..103): the binding declaration, the startup-gate
invocation, and the marked gate questions. The curator-agent delta
owns the agent-side mechanisms those requirements rely on (the
marker grammar, the gate tool, the permission model), so the distill
gate contract has exactly one owner per statement and later changes
cannot drift between two restatements. The capability's main spec is
not archived yet (`dewey-distill` is still pending), and OpenSpec
refuses a MODIFIED operation when no target spec exists, so the KD
delta uses ADDED requirements that reference the curator-agent
requirements by id instead of restating them.

Requirement-numbering coordination: this change allocates
KD-FR-101..103 and explicitly reserves KD-FR-001..099 for the pending
`dewey-distill` change's archive-time numbering (its requirements
currently carry names, not ids). Whichever change archives first,
the ranges cannot collide; the reservation is recorded in both delta
allocation headers.

### D6 Shipping and scaffold parity

Add a new `agents.go` with a `deweyAgentDefinitions` map (filename →
content) parallel to `deweySlashCommands`. In `cli.go`, inside the
existing `.opencode/` composability guard, scaffold
`.opencode/agents/curator.md` idempotently: create the directory and
file when absent, never overwrite.

Unlike the existing slash-command scaffolding (which logs only on
success and swallows write errors), the agent scaffold MUST surface
write failures: a failed or partial write would leave the shipped
`/dewey-distill` command with an unresolvable `agent: curator`
binding while `dewey init` reports success. The write uses
`O_CREATE|O_EXCL` (closing the stat-then-write TOCTOU window), and a
write error fails `dewey init` with the path named.

This is the smallest mechanism that makes the binding resolvable in
a vault initialized by `dewey init`. Without it, a scaffolded vault
would receive a command that names an agent the vault does not have.
`cli_test.go` gains assertions that the scaffolded bytes equal the
map entry and the tracked file, that an existing file is preserved,
that a write failure fails init, and that no `.opencode/agents/`
directory appears when `.opencode/` is absent — mirroring
`TestInitCmd_ScaffoldsDeweyDistillFromMap`.

The `dewey-distill.md` entry in `slash_commands.go` is updated in
lockstep with the tracked command file; the existing byte-identity
test therefore remains the guard for the command.

Ownership posture for `.opencode/agents/`: the directory is shared.
Dewey owns `curator.md`; the uf scaffolding toolchain owns its
`divisor-*` agents; neither removes or overwrites the other's files
(both scaffold only missing files). A manual check in the acceptance
matrix confirms a uf scaffold run leaves `curator.md` in place, and
the never-overwrite contract means a stale scaffolded curator is
updated deliberately by the user, matching the slash-command
contract.

### D7 Human-gate marker contract

A gate question begins with the exact token `HUMAN_GATE:` (uppercase,
ASCII colon) followed by one space and the question, at the start of
the first line with no leading whitespace. Example:

```
HUMAN_GATE: Which tags should I compile, all or the listed subset?
```

The contract is defined here, on the Dewey side, because the plugin
repositories are not readable from this workspace. Auto-answer
implementations that honor jflowers/opencode-auto-question#1 MUST
leave a marked question for a human. The command MUST NOT mark
informational lines, so a plugin can treat the prefix as a reliable
signal.

Structural discipline: the marker is asserted structurally, not by
global occurrence count. Each mandatory gate section of the command
carries exactly one marker-prefixed question line, and no line
outside the gate sections begins with the marker. Constraint prose
inside the command MUST NOT begin a line with the literal token (it
refers to "the human-gate marker" instead), so the structural
assertion cannot be tripped by documentation mentioning the token,
and a future legitimate third gate can add its own section without
failing a brittle count.

The marker grammar is published in the website documentation issue
(extended on unbound-force/website#371) so external implementers
have a stable public contract: uppercase token, single ASCII space,
start of the first line, no leading whitespace, never on
informational lines.

### D8 Deterministic auto-answer gate tool

The startup precondition is a fixed, judgment-free procedure —
config resolution, JSONC parsing, list comparison, prescribed output
— so per the determinism pack (DE-001, DE-003, DE-005) it lives in
an executable tool with tests, not in command prose. The tool is a
new CLI subcommand:

```
dewey gate auto-answer
```

A dedicated `gate` subcommand (rather than a `dewey doctor` check)
keeps the security verdict on its own exit code — 0 for pass, 1 for
refusal — which the command prose and scripts consume directly, and
keeps `doctor` focused on vault health. The tool renders its own
verdict output (DE-003); agents relay it verbatim rather than
reconstructing it.

Evaluation procedure (implemented in a new `gate.go`, unit-tested in
`gate_test.go` over fixture directories):

1. Resolve the plugin's config file: `auto-question.jsonc` in the
   OpenCode global config dir (`$XDG_CONFIG_HOME/opencode` when set,
   else `~/.config/opencode`). This is the plugin's single documented
   config location and the authoritative source of its `agents`
   list. A directory that cannot be resolved (both variables unset)
   makes the config unverifiable.
2. Determine existence. A stat that fails for any reason other than
   file-not-found (for example a permission error on the config
   directory) makes the config unverifiable. An absent file is the
   only path to the documented default.
3. If the file exists, parse it as JSONC (comments and trailing
   commas tolerated). Normalize every `agents` entry by trimming
   whitespace and case-folding. `{env:VAR}` interpolation applies
   only when the whole entry is exactly the interpolation token and
   is resolved against the process environment; a partial
   interpolation inside a larger entry, or an entry whose variable
   is unset, makes the list unverifiable.
4. Read the `agents` key. Absent key → documented default. Present
   but not an array of strings → unverifiable.
4a. Verify the curator binding resolves:
   `.opencode/agents/curator.md`, relative to the working
   directory, must exist and its frontmatter must declare
   `mode: subagent`. Missing or unparseable → REFUSE. This is
   what stops a host that ignores the `agent:` binding from
   running the command under the default auto-answer target
   (`autonomy`) with both defenses reporting green.
5. Verdict:
   - The effective list contains `curator` (after normalization) or
     the wildcard `*` → REFUSE, naming the config file, the
     offending entry, and the fix (remove the entry, then re-run
     `/dewey-distill`).
   - The config directory cannot be resolved, or a stat fails for a
     reason other than file-not-found → REFUSE (fail closed).
   - The file exists but cannot be read → REFUSE (fail closed).
   - The file exists but cannot be parsed → REFUSE (fail closed).
   - The `agents` value is not a list of strings, an entry's
     environment variable is unset, or an entry carries a partial
     interpolation → REFUSE (fail closed).
   - The curator definition is missing or unparseable → REFUSE.
   - Otherwise → PASS, reporting the config file checked, the
     effective agents list, whether the documented default applied,
     and that the curator binding resolved.
6. Registration scan (advisory context, reported in the verdict
   output): parse the project OpenCode configs (`.opencode/
   opencode.json[c]`, then `opencode.json[c]` at the repository
   root) and the global `opencode.json[c]` for `plugin` (V1) and
   `plugins` (V2) arrays, and check the plugin discovery
   directories (`.opencode/plugins/` and the global `plugins/`)
   for an `opencode-auto-question` or `auto-question` entry. An
   entry identifies the plugin when its string or `package` value
   has a final path segment equal to `opencode-auto-question`
   (case-insensitive), after stripping a leading `-` disable
   marker. The scan's result — registered, absent, or
   unreadable/unparseable (warning) — does not change the verdict:
   the plugin's own config file is the single documented source of
   the agents list, and registration state cannot change what the
   plugin would answer. A config the scan cannot read produces a
   warning line naming the file, never a silent pass.

Why the verdict keys on the plugin config rather than plugin
registration: the plugin answers questions only for session agents
in its configured `agents` list (verified from its README: "Session
agent in config `agents` list? NO → human answers"). A clean or
absent config file means the plugin cannot auto-answer the curator
whether or not it is registered, so a pass is sound even with
unknown registration; an unverifiable config file is exactly the
case where the gate must not proceed. Failing closed on the
authoritative source — rather than on every advisory input — also
keeps over-refusal bounded: a malformed project `opencode.json`
unrelated to the plugin produces a warning, not a blocked command.

`/dewey-distill` invokes the tool as its startup gate (see KD-FR-101)
and refuses on any verdict other than a pass, relaying the tool's
output verbatim. Because the tool is a user-level process, it reads
the global config directory directly — the curator agent itself
never reads outside the workspace, so the gate cannot be defeated by
an out-of-workspace permission ask that an unattended run would
stall on.

The frontmatter binding is what makes the active agent `curator`;
the gate does not re-derive host session state it cannot observe.
Instead the gate verifies the binding's *resolvability* — the
definition exists and parses — so a host that cannot honor the
binding is refused rather than silently running under the session
agent.

### D9 Two gates, one marker

The scope gate and the review gate both use `HUMAN_GATE:`. The
existing prohibition on calling `compile`/`curate` before scope
approval and `promote` before review approval stays; the marker makes
the question visible to auto-answer suppression. The command's
existing prompt-mode and lock-safety behavior is unchanged.

### D10 No regression

No existing agent file's `tools` flags, `permission` rules, or `mode`
change. No existing command binding changes. The new scaffolding
runs only when `.opencode/` already exists and only creates a missing
file, so it cannot overwrite user customization. The repository's
`opencode.json` gains nothing: the bash carve-out ships inside the
agent definition, not in committed project config.

### D11 Manual acceptance matrix

The Go test suite does not run OpenCode, so host-dependent behavior
is enumerated as one manual matrix in `tasks.md`:

- the command binding switches the agent from a development session
  and restores it afterward;
- a session whose `agent: curator` binding cannot be resolved (the
  definition is removed) is refused by the gate instead of running;
- `dewey gate auto-answer` runs under the curator (the carve-out
  works) and its PASS/REFUSED output renders as prescribed;
- `git commit`, `git push`, and an arbitrary shell command are
  refused under the curator;
- the adversarial shell forms are refused under the installed
  host: `dewey gate auto-answer && <cmd>`,
  `dewey gate auto-answer; <cmd>`, and
  `dewey gate auto-answer $(<cmd>)`, plus a leading
  `XDG_CONFIG_HOME` assignment prefix (the last recorded, not
  asserted, since it depends on argv-versus-string matcher
  semantics);
- a subagent spawn is refused under the curator;
- an allowlisted MCP tool call (for example `lint`) succeeds under
  the curator, and a requested non-allowlisted mutation (for example
  `delete_page`) is refused by the curator citing its allowlist;
- both gate questions render with the `HUMAN_GATE: ` prefix, and an
  auto-answer plugin honoring the external contract leaves them for
  a human;
- a uf scaffold run leaves `.opencode/agents/curator.md` in place.

## Assumptions

### A1 Auto-answer plugin configuration shape (verified)

The opencode-auto-question plugin's configuration was verified
against its published README during spec review. The plugin is
registered through the OpenCode `plugin`/`plugins` array or
auto-discovery directories, and reads its own config from
`auto-question.jsonc` in the OpenCode global config dir — JSONC with
`{env:VAR}` interpolation — where an `agents` key lists the session
agents it auto-answers, defaulting to `["autonomy"]`. The gate tool
(D8) is specified against this documented schema. If the plugin's
schema changes, only the gate tool and this assumption need
updating; the change's tests pin the current contract.

### A2 Permission model (verified)

OpenCode's V1 permission model supports per-tool and per-command
rules, including nested `bash` command maps, and denies `question`
for custom agents unless allowed. A `tools:` block disables only the
tools it lists; unlisted tools stay enabled, which is why the
curator sets `bash: true` explicitly and narrows it at the
permission layer rather than relying on defaults. V1 cannot scope
individual MCP tools; V2's `permissions` rule list can (including
`<server>_<tool>` actions) and is recorded as the conversion path in
D3. Each agent file is read as exactly one format.

### A3 Host-side agent switch

The `agent:` command frontmatter switches the active agent for the
command's duration and restores the prior agent afterward, per the
issue's acceptance criterion. This is verified manually because the
Go test suite does not run OpenCode.

### A4 Agent scaffolding is the shipping mechanism

Dewey ships reference agents by tracking them in `.opencode/agents/`
and by scaffolding them through `dewey init`, parallel to slash
commands. No other deployment mechanism exists in this repository,
and adding the agent only to the tracked tree would leave scaffolded
vaults with a dangling binding.

### A5 Marker spelling is the contract

`HUMAN_GATE:` is the canonical token. If the external plugin settles
on different spelling, the change updates this document, the command
prose, and the spec together; there is no second source.

## Risks / Trade-offs

- **External contract drift.** A plugin may not honor the marker, or
  its config schema may change. Mitigation: the startup gate blocks
  the known auto-answer configuration independently of marker
  handling and fails closed when its config cannot be verified; the
  gate's tests pin the documented schema so drift surfaces as a
  failing test, not a silent bypass.
- **Unknown auto-answer mechanisms.** The gate verifies only the
  known plugin's config, and the marker is honored only by plugins
  implementing the external contract. A fork, a renamed copy, or a
  different vendor's question-answering plugin reads neither input,
  so both defenses are bypassed by construction. Residual risk is
  accepted and stated: this change hardens the known plugin path,
  not an open-ended set of unknown mechanisms.
- **Carve-out depends on host support.** If a host ignores nested
  `bash` permission maps, the curator's shell would be broader than
  specified. Mitigation: the acceptance matrix verifies `git commit`,
  `git push`, an arbitrary command, and the adversarial forms above
  are refused under the installed host. Under the intended deployment
  (curator guards relaxed from global config) the bash map is the
  primary hard shell boundary, so the matrix — not the supervisor —
  is the acceptance evidence.
- **MCP allowlist is policy, not host enforcement.** V1 cannot
  hard-scope MCP tools, so a misbehaving curator could call a
  non-allowlisted tool. Mitigation: the allowlist is stated in the
  agent body, asserted on the tracked files by tests, verified in the
  acceptance matrix, and converted to hard V2 `permissions` rules
  when the repository migrates agent files.
- **Over-refusal.** An unreadable or malformed plugin config stops
  the command even when auto-answering might be off. Accepted: the
  gate either verifies or does not run; the refusal names the file
  and the fix. Advisory registration-scan failures produce warnings,
  not refusals, to keep the fail-closed surface bounded to the
  authoritative config.
- **Stale scaffolded agent.** `dewey init` never overwrites an
  existing agent file, so a user with an old copy must update it
  deliberately. Accepted to preserve user customization, matching the
  slash-command contract.
- **No direct artifact writes.** If a future workflow step needs a
  local file outside `.uf/dewey/`, the curator cannot write it. That
  is a deliberate boundary and would require a new spec to widen.
- **Manual host verification.** The binding, the permission
  carve-out, and the marker behavior depend on OpenCode behavior the
  Go tests cannot exercise. Accepted; the matrix is enumerated in
  D11 and `tasks.md`.

## Constitution Alignment

The proposal records PASS for all four principles. This design
preserves them: the gate tool and agent scaffolding add no mandatory
dependency when no plugin is present (I), the agent, marker, and gate
verdict are declared artifacts rather than runtime coupling (II),
refusals name configuration and fix with tool-rendered, tested output
(III), and the gate matrix, scaffolding, marker structure, and
allowlist are unit-tested while host-dependent behavior is enumerated
as manual acceptance (IV).
