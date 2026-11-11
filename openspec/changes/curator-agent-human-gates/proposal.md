<!-- originating_issue: 152 -->
## Why

`/dewey-distill` already runs lint, compile, curate, and promote through
the running MCP server with two mandatory human gates (change
`dewey-distill`, #145). It runs under whatever agent the session
started with. Corpus-scale curation then trips the supervisor's
reward-hacking and path-trace guards: the run quotes `--no-verify` and
names hundreds of paths. The workaround used so far is to commit
`blockRewardHacking: false` and `requireStepReferenceInTools: false`
into project config. That workaround is unsafe the moment it is copied
into a repository used for ordinary development, because it disables
guards for every agent in that repository.

A dedicated, narrowly permitted curator agent makes narrowed guards
safe. The agent gets its own identity, the command binds to it, and
the guard relaxation it needs is granted from the user's global
supervisor configuration rather than from committed project config.

The same command must also keep its gates human. An automatic
question-answering plugin can currently answer the scope and review
questions before the human sees them, which turns an approval gate
into a rubber stamp. The command therefore needs a deterministic
startup gate and a question marker that auto-answer plugins honor.

Part of #153 (umbrella). Depends on #145 (merged). Pairs with
jflowers/opencode-supervisor#48 (per-agent guard overrides) and
jflowers/opencode-auto-question#1 (human-gate marker).

## What Changes

- **Curator agent definition** at `.opencode/agents/curator.md`,
  following the existing V1 agent frontmatter shape: a `description`,
  `mode: subagent`, a `tools:` block, and a `permission:` block.
- **Layered least-privilege capability boundary**: the `tools:` block
  grants `read` and denies `write`, `edit`, and `webfetch` outright.
  `bash` is enabled at the tools layer and narrowed at the permission
  layer to exactly one command — `dewey gate auto-answer` — because
  the startup gate (below) is a CLI tool; every other shell command is
  denied. The `permission:` block also denies `task` (no subagent
  spawning), `edit`, `write`, `patch`, and `webfetch`, and allows
  `question` so the two human gates remain askable. No `git commit`,
  `git push`, or arbitrary shell; no source, test, or config edits.
- **Dewey MCP tool allowlist**: the curator's Dewey MCP surface is
  limited to exactly `lint`, `list_pages`, `compile`, `curate`,
  `store_compiled`, and `promote` — the tools the distill pipeline
  needs. The agent body states the allowlist and forbids every other
  MCP tool (page/block mutation, deletion, renaming, learning writes).
  Persisted writes flow only through the allowlisted tools, whose
  server-side destinations are stated per tool: `compile` and
  `store_compiled` under `.uf/dewey/compiled/`, `promote` as a store
  database tier change, and `curate` under each knowledge store's
  configured `path` (default `.uf/dewey/knowledge/{name}`,
  user-configurable and possibly outside `.uf/dewey/`). The current
  V1 agent format cannot
  hard-scope individual MCP tools, so the allowlist is enforced by the
  agent's stated policy, the command's tool discipline, tracked-file
  assertions, and the manual acceptance matrix; the design records the
  V2 `permissions` conversion as the future hard-enforcement path.
- **Command binding**: `/dewey-distill` declares `agent: curator` in
  its frontmatter, the same pattern `gaze.md` uses for
  `gaze-reporter`. Normal development sessions never run under the
  curator.
- **Deterministic auto-answer gate tool**: a new `dewey gate
  auto-answer` CLI subcommand (backed by a new `gate.go` with
  `gate_test.go`) that resolves the opencode-auto-question plugin's
  real configuration — its own config file at the OpenCode global
  config dir (`auto-question.jsonc`, JSONC with `{env:VAR}`
  interpolation, `agents` key, documented default `["autonomy"]`) —
  plus advisory plugin-registration scanning of project and global
  OpenCode configs. It prints a tool-rendered verdict (exit 0 on pass,
  1 on refusal) and fails closed whenever the plugin config exists but
  cannot be read, parsed, or interpreted. `/dewey-distill` runs this
  one command at startup and refuses on any non-pass verdict.
- **Human-gate marker contract**: every scope-gate and review-gate
  question begins with the marker token `HUMAN_GATE:`. The marker
  grammar is defined on the Dewey side and published in the website
  documentation issue so external implementers have a stable contract.
  Auto-answer plugins that implement jflowers/opencode-auto-question#1
  MUST treat a marked question as human-only; the plugin-side behavior
  is an external contract, not something this repository implements.
- **Shipping**: `dewey init` scaffolds the agent definition into
  `.opencode/agents/` when `.opencode/` exists, idempotently, never
  overwriting an existing file, and surfacing write failures as init
  errors rather than silently succeeding. The repository-tracked agent
  file and the shipped definition stay byte-identical.
- **No guard relaxation in this repository**: project config gains no
  supervisor guard override. Any relaxation for the curator is
  granted in the user's global supervisor configuration.

## Capabilities

### New Capabilities
- `curator-agent`: the curator identity, its layered capability
  boundary, its MCP tool allowlist, the human-gate marker grammar, the
  `dewey gate auto-answer` tool, agent shipping and scaffolding, and
  the no-committed-relaxation rule.

### Modified Capabilities
- `knowledge-distillation`: the `/dewey-distill` command declares an
  agent binding, invokes the auto-answer gate tool at startup and
  refuses on a non-pass verdict, and marks both mandatory gate
  questions as human-only. The capability has no archived main spec
  yet, so its delta is expressed as ADDED requirements (KD-FR-101
  through KD-FR-103) alongside the pending "Gated Distill Workflow"
  contract from the `dewey-distill` change.

### Removed Capabilities
- None.

## Impact

- `.opencode/agents/curator.md` (new) — shipped agent definition.
- `agents.go` (new) — shipped agent-definition map.
- `gate.go` (new) — deterministic auto-answer gate evaluation
  (config resolution, JSONC parsing, agent-list comparison,
  registration scan, verdict).
- `cli.go` — scaffold the curator agent into `.opencode/agents/`
  when `.opencode/` exists; register the `dewey gate auto-answer`
  subcommand.
- `.opencode/commands/dewey-distill.md` — `agent: curator`
  frontmatter, a startup-gate section that runs the gate tool, and
  marked gate questions.
- `slash_commands.go` — the `dewey-distill.md` scaffold entry stays
  byte-identical to the tracked command file.
- Tests: `cli_test.go` (scaffolding, byte-identity, marker-structure,
  and allowlist assertions) and `gate_test.go` (the full
  pass/refuse/fail-closed matrix over fixture configs).
- `AGENTS.md` and `README.md` — document the curator agent, its
  permission model and MCP allowlist, the `dewey gate auto-answer`
  command, the new `dewey init` scaffolding artifact, and the human
  gates. This repository has no agent inventory section; the
  documentation update lands in the existing distillation text, the
  CLI command references, and Recent Changes.
- Website tracking: the open docs issue unbound-force/website#371
  already covers the `/dewey-distill` workflow and both human gates,
  so the change extends it (comment) with the curator agent, the
  permission model, the gate command, and the published marker
  grammar rather than filing a duplicate.
- Content opportunity (informational): a new agent identity plus a
  human-gate workflow pattern is above the blog significance
  threshold; a blog or tutorial post covering the curator workflow is
  a natural follow-up and is recorded here so it is not lost.
- Go production code changes are limited to `gate.go`, `cli.go`
  (scaffolding + subcommand registration), `agents.go`, and
  `slash_commands.go`. No MCP tool contract or database schema
  changes; no new module dependencies.

**Out of scope:** implementing the auto-answer plugin's marker
handling (external, jflowers/opencode-auto-question#1), implementing
per-agent supervisor overrides (external,
jflowers/opencode-supervisor#48), removing or weakening the
supervisor guards themselves, and hard-scoping individual MCP tools
via V2 `permissions` rules (recorded as a design note for the
repository's future V2 agent-file migration).

## Constitution Alignment

Assessed against the Dewey constitution (v1.4.0).

### I. Composability First

**Assessment**: PASS

The curator agent and the gate tool add no mandatory dependency.
Without the auto-question plugin the gate passes on the documented
default configuration, and the command behaves as it does today. Dewey
still installs and operates without OpenCode; the agent file is
scaffolded only when `.opencode/` already exists.

### II. Autonomous Collaboration

**Assessment**: PASS

The binding, marker grammar, and gate verdict are declared artifacts
(agent frontmatter, command frontmatter, a documented marker token, a
tested CLI tool). The external plugin contract is referenced by issue,
not implemented through runtime coupling.

### III. Observable Quality

**Assessment**: PASS

A refusal names the offending configuration and the corrective
action, and no pipeline stage runs. The gate tool's verdict output is
tool-rendered and covered by tests. The agent definition, the command
frontmatter, and the shipped-definition map are auditable at rest and
kept byte-identical by tests.

### IV. Testability

**Assessment**: PASS

Scaffolding is covered by tests that assert the created file equals
the shipped definition, that an existing file is preserved, and that a
write failure fails `dewey init` with a named path. The auto-answer
gate's pass/refuse/fail-closed matrix is covered by unit tests over
fixture configurations. The agent binding, the permission carve-out,
and the host-side marker behavior depend on the OpenCode host, which
the Go test suite does not run, so they are enumerated as explicit
manual acceptance checks in `tasks.md`.

### Development Standards Protocol — CLI-subcommand exposure

**Assessment**: PASS (documented exception)

The constitution's Development Standards Protocol says new
capabilities MUST be exposed as MCP tools, not as CLI subcommands.
`dewey gate auto-answer` is deliberately a CLI subcommand: the
startup gate is a local, pre-session precondition rather than a
knowledge operation, and it needs its own exit code for command
prose to consume (the determinism pack's DE-003 exit-code design).
It follows the repository's operational-command precedent
(`dewey doctor`, `dewey manifest`, `dewey artifacts`), none of
which are MCP tools. The trade-off is acknowledged here rather
than left implicit; an MCP tool was rejected because the gate must
run as a pre-session process, ahead of the MCP-dependent pipeline
it authorizes. Because the Protocol clause is a MUST, this
deviation is presented for explicit human ratification as part of
this change's review; it is not a standing amendment and does not
weaken the clause for future capabilities.
