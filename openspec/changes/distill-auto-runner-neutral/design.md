## Context

`/dewey-distill-auto` runs as the `distiller` agent. Its second startup
gate, `dewey gate distill-unattended` (`gate.go`,
`evaluateDistillUnattendedGate`), enforces FR-016: unattended mode is
enabled only when the user's *global* opencode-supervisor config carries
a command-scoped `dewey-distill-auto` profile. Today it probes
`$HOME/.config/opencode-supervisor/supervisor.yaml` plus the XDG
equivalent for a YAML `commands.dewey-distill-auto` key — a placeholder
written before jflowers/opencode-supervisor#51 existed. On refusal it
records a no-op artifact (`recordSupervisorNoOp`) and renders a
`Notice:` line that the command posts on the dispatch issue with
`gh issue comment <N>`.

The dispatch issue is opened by the scaffolded GitHub Actions template
(`workflow_templates.go` → `.github/workflows/dewey-distill.yml.example`,
written by `dewey init` through `scaffoldToolOwnedFiles`), and a
dispatcher outside Dewey turns that issue into `/dewey-distill-auto #N`.

opencode-supervisor v2.9.1 shipped #51 with a JSON config at a different
path (see A1).

## Goals / Non-Goals

### Goals

- The gate passes exactly when the supervisor would apply a
  `dewey-distill-auto` command profile, and refuses fail-closed
  otherwise.
- `/dewey-distill-auto` is runnable by any scheduler or agent runner; no
  issue, workflow or named dispatcher is part of Dewey's contract.
- The distiller loses its only GitHub write capability
  (`gh issue comment`).

### Non-Goals

- Validating profile contents beyond `match.command` (`traceCheck` and
  the rest are the supervisor's contract).
- Honoring `match.agent`-only profiles (the requirement is
  command-scoped).
- Removing previously scaffolded workflow examples from adopting
  repositories (D6).
- Rewriting `specs/018-*` or earlier OpenSpec changes.

## Decisions

### D1 One candidate file, resolved the way the supervisor resolves it

`supervisorConfigPath(lookup)`:

1. `XDG_CONFIG_HOME` set, non-blank (after trimming) and
   `filepath.IsAbs` → `$XDG_CONFIG_HOME/opencode/supervisor/config.json`.
   HOME is not consulted.
2. Otherwise (unset, blank or relative XDG — ignored, not refused) →
   `$HOME/.config/opencode/supervisor/config.json`, where HOME must be set
   and absolute; else the gate refuses with a resolution error.

There is exactly one candidate, so a clean file at one path can never
mask another, and the verdict names that one file (`Checked:`) on
refusal and (`Config file:`) on pass. Unlike the old probe, a relative
XDG no longer refuses: the supervisor ignores it, so the gate must
follow the home path the supervisor will actually use.

The injected `gateOptions.supervisorConfigDir` seam is kept and now
means "the supervisor's config directory": the gate checks
`<dir>/config.json`.

### D2 Profile match

The file is parsed with `encoding/json` into a top-level
`map[string]json.RawMessage`.

- `profiles` absent or `null` → no profile → refuse with
  `NoOpReasonSupervisorUnavailable` (the config exists but does not
  enable unattended distill).
- `profiles` present and not an array → refuse fail-closed
  ("`profiles` is not an array"), naming the file.
- Each array element is decoded leniently: an element that is not an
  object, or whose `match` / `match.command` is missing or not a string,
  simply does not match. Only a match is decisive; malformed siblings
  of a matching profile do not refuse, mirroring the supervisor's
  find-first-match behavior.
- `match.command` matches when `strings.TrimPrefix(strings.TrimSpace(c),
  "/") == "dewey-distill-auto"`.

Invalid JSON (including JSONC comments or trailing commas) refuses: the
supervisor's `JSON.parse` would reject the same file.

### D3 Refusal reasons and fixes

| Condition | Reason | Fix |
|---|---|---|
| file absent | `supervisor command profile unavailable` | add the profile (JSON snippet path named) |
| read error (not ENOENT) | config exists but cannot be read | make the file readable |
| invalid JSON | not valid JSON | fix the JSON syntax |
| `profiles` not an array | `profiles` is not an array | make `profiles` an array |
| no matching profile | `supervisor command profile unavailable` | add the profile |
| HOME unresolvable | cannot resolve the supervisor config location | set HOME (or an absolute XDG_CONFIG_HOME) |

The "add the profile" Fix names the file and the minimal profile
`{"profiles":[{"name":"distill","match":{"command":"dewey-distill-auto"}}]}`.
Every refusal still writes the no-op artifact with
`NoOpReasonSupervisorUnavailable` (unchanged `runDistillUnattendedGate`).

### D4 Verdict without `Notice:`

`renderDistillUnattendedVerdict` drops the `Notice:` line and the
`supervisorUnavailableNotice` constant. The artifact path line stays.
The command, on refusal, stops and relays the verdict; nothing is posted
anywhere.

### D5 Command and agent text; scaffold refresh

The embedded `deweySlashCommands["dewey-distill-auto.md"]` and
`deweyAgentDefinitions["distiller.md"]` lose the `#N` argument, the
dispatcher line, the issue-comment step and the
`"gh issue comment": allow` entry. The tracked copies under `.opencode/`
are regenerated from the embedded payloads (same marker mechanism,
`dev` version) so the existing parity tests keep them byte-identical.
Because the tool-owned scaffold mechanism refreshes untouched copies
whose marker digest matches their payload, adopting repositories pick
up the new text on the next `dewey init`; customized copies are
preserved as before.

### D6 Workflow template removal; no retirement path (follow-up)

`workflow_templates.go`, its test and the tracked example are deleted
and `dewey init` stops scaffolding `.github/workflows/`. The scaffold
mechanism (`scaffoldToolOwnedFiles` / `classifyScaffold`) has no
"retired file" path, and building one is out of scope. Adopting
repositories that ran an earlier `dewey init` keep an inert
`.github/workflows/dewey-distill.yml.example` until they delete it; a
copied live `dewey-distill.yml` keeps opening issues nobody needs and
should be deleted by hand. Follow-up: a retired-scaffold list that
removes untouched (digest-matching) files on re-init.

### D7 Documentation neutrality

README and AGENTS.md describe scheduling as "run
`/dewey-distill-auto` from any scheduler or agent runner, in a clean
checkout, with the global supervisor profile configured", with the JSON
snippet. Code comments that named a dispatcher are reworded neutrally.
Historical `specs/`, `openspec/changes/` and `.uf/dewey/learnings/` are
left as written.

## Risks / Trade-offs

1. **Supervisor contract drift.** If the supervisor changes its path or
   shape again, the gate refuses (fail-closed) until updated — the same
   posture as before, now pinned to a shipped version (A1).
2. **Lenient element decoding.** A malformed sibling profile next to a
   valid match passes. Accepted: the supervisor would apply the match.
3. **Orphaned example files** in adopting repositories (D6).
4. **Lost notice channel.** Refusals are no longer announced on an
   issue; the no-op artifact and the runner's own output are the audit
   surface. A runner that wants notifications can key on the gate's exit
   code.

## Constitution Alignment

See proposal.md: Composability First PASS, Autonomous Collaboration
PASS, Observable Quality PASS, Testability PASS. The design removes an
external trigger dependency, reads the supervisor's config read-only,
keeps the verdict-plus-artifact audit surface, and is covered through
the existing seams.

## Assumptions

### A1 Supervisor config contract (opencode-supervisor v2.9.1, #51)

Global file only; `$XDG_CONFIG_HOME/opencode/supervisor/config.json`
when XDG is set, non-blank and absolute, else
`$HOME/.config/opencode/supervisor/config.json`; strict JSON; `profiles`
array of objects with `match.command` (trimmed, leading `/` stripped)
and/or `match.agent`. Repo-local profiles never honored.

### A2 `HOME` is the supervisor's home directory

The supervisor's home lookup resolves to `$HOME` when set, as for the
auto-answer gate (gate-config-sources-parity A2).

### A3 The gate and supervisor share one environment

As with the auto-answer gate, a supervisor process with a different
`XDG_CONFIG_HOME` or `HOME` than the gate process reads a different
file. The command runs both in the same session, so this is the normal
case; divergence is a documented residual.
