## Why

Unattended distill (`018-weekly-distill-automation`) cannot run on any
machine today, and its trigger couples Dewey to one external dispatcher.

**1. The startup gate reads a file the supervisor never wrote.**
`dewey gate distill-unattended` is the fail-closed gate that keeps
`/dewey-distill-auto` from running unless the user's *global*
opencode-supervisor config carries the command-scoped
`dewey-distill-auto` profile (FR-016). It looks for
`~/.config/opencode-supervisor/supervisor.yaml` (and
`$XDG_CONFIG_HOME/opencode-supervisor/supervisor.yaml`) with a top-level
`commands:` map keyed `dewey-distill-auto`. That location and shape were
a guess written before jflowers/opencode-supervisor#51 shipped — the
`supervisorConfigDirName` comment in `gate.go` says to update the
candidates once #51 ships.

#51 shipped in opencode-supervisor v2.9.1 with a different location and
shape:

- The supervisor reads profiles **only** from its global config file:
  `$XDG_CONFIG_HOME/opencode/supervisor/config.json` when
  `XDG_CONFIG_HOME` is set, non-blank and absolute; otherwise
  `$HOME/.config/opencode/supervisor/config.json`. A relative or blank
  `XDG_CONFIG_HOME` is ignored, the same as unset. Profiles in
  repo-local config are never honored.
- The file is strict JSON (`JSON.parse`).
- Shape:

  ```json
  {
    "profiles": [
      {
        "name": "distill",
        "match": { "command": "dewey-distill-auto" },
        "traceCheck": { "exemptTools": ["dewey_*", "read", "grep", "glob"] }
      }
    ]
  }
  ```

  `match` keys are `command` (slash-command name, normalized by trimming
  and stripping a leading `/`) and/or `agent`.

So the gate refuses on every machine even when the real profile is
configured: unattended distill can never run.

**2. The trigger is convoluted and names a personal tool.** The weekly
trigger is a GitHub Actions template
(`.github/workflows/dewey-distill.yml.example`, embedded in
`workflow_templates.go`, scaffolded by `dewey init`) that opens or
re-triggers a `dewey-distill` issue so that one specific dispatcher
(gh-hud, a personal project) runs `/dewey-distill-auto #N`. Dewey must
not depend on or name a particular dispatcher: tools may conform to
Dewey, not the other way round. The issue also carries nothing the run
needs — its number is used only to `gh issue comment` the refusal
notice, but a refusal already records a no-op artifact under
`.uf/dewey/distill-runs/`, and a real run's output is the rolling
`dewey-distill/<theme>` PRs.

## What Changes

- **Gate reads the supervisor's real config.** `dewey gate
  distill-unattended` checks exactly the one file the supervisor reads:
  `$XDG_CONFIG_HOME/opencode/supervisor/config.json` when
  `XDG_CONFIG_HOME` is non-blank and absolute, else
  `$HOME/.config/opencode/supervisor/config.json`. It passes iff that
  file parses as JSON and `profiles` is an array containing an object
  whose `match.command`, normalized (trim, strip leading `/`), equals
  `dewey-distill-auto`. Absent file → refuse with the existing
  supervisor-unavailable reason. Unreadable file, invalid JSON, or a
  `profiles` key that is not an array → refuse fail-closed, naming the
  file. The Fix text says how to add the profile. The no-op run artifact
  on refusal is unchanged.
- **No dispatch issue.** The rendered verdict drops the `Notice:` line
  and the notice-comment concept. `NoOpReasonSupervisorUnavailable` stays
  in the artifact.
- **Command and agent take no trigger argument.** The embedded
  `dewey-distill-auto` command and `distiller` agent (and their tracked
  scaffold copies) drop the `#N` issue argument, the dispatcher line and
  the "post the Notice as a comment on the dispatch issue" step; a gate
  refusal stops the run and relays the verdict. `gh issue comment` is
  removed from the distiller's bash allowlist. Usage stays
  `/dewey-distill-auto [--dry-run] [--backlog-from DATE]`; a stray
  trailing argument is ignored, so callers still passing `#N` keep
  working.
- **Workflow template removed.** `workflow_templates.go`,
  `workflow_template_test.go`, `.github/workflows/dewey-distill.yml.example`
  and the `dewey init` scaffolding step are deleted.
- **Runner-neutral docs.** README and AGENTS.md describe scheduling
  neutrally: run `/dewey-distill-auto` from any scheduler or agent runner,
  on any cadence, in a clean checkout, with the global supervisor profile
  configured. Every dispatcher-specific reference outside historical
  specs, OpenSpec changes and stored learnings is removed.

## Capabilities

### New Capabilities

- None.

### Modified Capabilities

- `weekly-distill`: the requirements of Speckit spec
  `specs/018-weekly-distill-automation/spec.md` — FR-001, FR-004, FR-016,
  FR-022 and the opencode-supervisor#51 Assumption are modified; FR-002,
  FR-003 and SC-008 are removed. The historical spec is not rewritten;
  this change records the decision.

### Removed Capabilities

- None (the scheduled-issue trigger is a requirement removal inside
  `weekly-distill`, not a capability of its own).

## Impact

- `gate.go`, `gate_test.go` — config resolution, JSON profile match,
  verdict rendering, cobra help, GoDoc.
- `slash_commands.go`, `.opencode/commands/dewey-distill-auto.md`,
  `agents.go`, `.opencode/agents/distiller.md` — command/agent text,
  allowlist; scaffold markers regenerated.
- `cli.go`, `cli_test.go`, `scaffold_test.go` — workflow scaffolding and
  its assertions removed; allowlist tests updated.
- Deleted: `workflow_templates.go`, `workflow_template_test.go`,
  `.github/workflows/dewey-distill.yml.example`.
- `tools/rolling_pr.go` (comment), `tools/distill_e2e_test.go`,
  `tools/distill_edge_cases_test.go`, `tools/rolling_pr_test.go`
  (comments only).
- `README.md`, `AGENTS.md`.
- No MCP tool contract change, no schema change, no new dependency
  (`encoding/json` only; `gopkg.in/yaml.v3` stays for other users).

**Out of scope:** retiring an already-scaffolded, untouched
`.github/workflows/dewey-distill.yml.example` in adopting repositories on
re-init (the tool-owned scaffold mechanism has no removal path; recorded
as a follow-up in design D6); validating the profile's `traceCheck`
contents (the supervisor's contract, not the gate's); honoring
`match.agent` profiles.

## Constitution Alignment

Assessed against the Dewey constitution (v1.5.0).

### I. Composability First

**Assessment**: PASS

The change removes a dependency on a named external dispatcher and a
GitHub Actions trigger. `/dewey-distill-auto` becomes runnable from any
scheduler or agent runner; Dewey stays installable and usable alone, and
interactive `/dewey-distill` is untouched.

### II. Autonomous Collaboration

**Assessment**: PASS

The gate collaborates with opencode-supervisor only through the
supervisor's documented config artifact, read-only, with a tool-rendered
verdict plus exit code as its contract. No runtime coupling is added;
the issue-comment side channel is removed.

### III. Observable Quality

**Assessment**: PASS

Every refusal still names the checked file, the reason and the fix, and
still records the no-op run artifact under `.uf/dewey/distill-runs/` —
which becomes the sole refusal audit surface instead of an issue comment.

### IV. Testability

**Assessment**: PASS

All gate behavior is exercised through the existing injectable
`lookupEnv` and `supervisorConfigDir` seams against `t.TempDir()` trees;
no supervisor install, network or GitHub access is needed.
