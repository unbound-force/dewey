# Delta Spec: weekly-distill

<!--
  Requirement numbering
  =====================
  Capability: weekly-distill. Its requirements live in the Speckit spec
  specs/018-weekly-distill-automation/spec.md (FR-001 .. FR-022,
  SC-001 .. SC-0xx, Assumptions); there is no archived OpenSpec main
  spec. This delta modifies and removes requirements by their Speckit
  ids and allocates no new ids. The historical spec file is not
  rewritten; this change records the decision.
-->

## ADDED Requirements

_None._

## MODIFIED Requirements

### Requirement: Runner-Neutral Scheduling

**ID**: FR-001 (replaces "reusable GitHub Actions template workflow")

Dewey MUST NOT ship, scaffold or require a scheduled trigger for
unattended distill. `/dewey-distill-auto` MUST be a self-contained
command that any scheduler or agent runner can invoke on any cadence in
a clean checkout of the knowledge repository. `dewey init` MUST NOT
write any file under `.github/workflows/`.

#### Scenario: init scaffolds no workflow

- **Given** a vault with `.opencode/` present
- **When** the user runs `dewey init`
- **Then** no file is created under `.github/workflows/`
- **And** the `dewey-distill-auto` command and `distiller` agent are
  scaffolded as before

#### Scenario: any runner can start a run

- **Given** a clean checkout with the global supervisor profile configured
- **When** a scheduler or agent runner invokes `/dewey-distill-auto`
- **Then** the run proceeds through the startup gates to scope, grouping
  and rolling PRs without needing an issue, issue number or workflow

### Requirement: Invocation Contract

**ID**: FR-004 (replaces "integrate with a named dispatcher")

Dewey MUST NOT depend on, integrate with, or name any specific
dispatcher. The only invocation contract Dewey exposes is the command
text `/dewey-distill-auto [--dry-run] [--backlog-from YYYY-MM-DD]`. The
command MUST NOT require a trigger argument and MUST ignore a stray
trailing argument (such as `#N`), so existing callers keep working. The
command MUST NOT post comments to any issue.

#### Scenario: legacy issue argument is ignored

- **Given** a runner that still invokes `/dewey-distill-auto #42`
- **When** the command starts
- **Then** `#42` is ignored and the run behaves exactly as
  `/dewey-distill-auto`

#### Scenario: refusal is relayed, not posted

- **Given** `dewey gate distill-unattended` refuses
- **When** the command handles the refusal
- **Then** it stops before scope and relays the verdict
- **And** it posts nothing to any issue

### Requirement: Supervisor Profile Gate

**ID**: FR-016

The distiller agent (`/dewey-distill-auto`) MUST operate with a narrowed
tool allowlist granted only from the user's global opencode-supervisor
config via a command-scoped profile for `dewey-distill-auto`.
Repository-local config MUST NOT be able to widen the allowlist.

`dewey gate distill-unattended` MUST check exactly one file — the one
the supervisor reads:

1. `$XDG_CONFIG_HOME/opencode/supervisor/config.json` when
   `XDG_CONFIG_HOME` is set, non-blank and an absolute path;
2. otherwise `$HOME/.config/opencode/supervisor/config.json`. A blank or
   relative `XDG_CONFIG_HOME` MUST be ignored, the same as unset.

The gate MUST pass if and only if that file parses as strict JSON and
its top-level `profiles` value is an array containing an object whose
`match.command`, after trimming whitespace and stripping one leading
`/`, equals `dewey-distill-auto`. It MUST NOT read any repository-local
file. Every other outcome MUST refuse fail-closed, exit 1, record the
no-op run artifact with `no_op_reason: supervisor command profile
unavailable` under `.uf/dewey/distill-runs/`, and render a verdict that
names the checked file, a reason and a fix:

- absent file, or no matching profile → reason `supervisor command
  profile unavailable`, fix naming the file and the profile to add;
- unreadable file, invalid JSON, a non-object top level, or `profiles`
  present but not an array → a reason naming the defect, fix naming the
  file;
- `HOME` unset or relative when the home path is needed → a resolution
  reason.

The verdict MUST NOT carry a notice-comment line.

#### Scenario: configured profile passes

- **Given** `$HOME/.config/opencode/supervisor/config.json` contains
  `{"profiles":[{"name":"distill","match":{"command":"dewey-distill-auto"}}]}`
- **And** `XDG_CONFIG_HOME` is unset
- **When** `dewey gate distill-unattended` runs
- **Then** it exits 0 and the verdict names that file

#### Scenario: leading slash is normalized

- **Given** the profile's `match.command` is `" /dewey-distill-auto "`
- **When** the gate runs
- **Then** it passes

#### Scenario: absent config refuses

- **Given** no supervisor config file exists
- **When** the gate runs
- **Then** it exits 1 with reason `supervisor command profile unavailable`
- **And** a no-op run artifact is recorded
- **And** the verdict has no `Notice:` line

#### Scenario: XDG takes precedence

- **Given** `XDG_CONFIG_HOME` is an absolute path whose
  `opencode/supervisor/config.json` has no matching profile
- **And** the home-directory file has a matching profile
- **When** the gate runs
- **Then** it refuses, naming the XDG file

#### Scenario: relative XDG is ignored

- **Given** `XDG_CONFIG_HOME=relative/dir`
- **And** the home-directory file has a matching profile
- **When** the gate runs
- **Then** it passes, naming the home-directory file

#### Scenario: invalid JSON refuses

- **Given** the config file is not valid JSON
- **When** the gate runs
- **Then** it refuses fail-closed with a reason naming invalid JSON and
  a fix naming the file

#### Scenario: profiles not an array refuses

- **Given** the config file is `{"profiles": {"distill": {}}}`
- **When** the gate runs
- **Then** it refuses fail-closed, naming the file

#### Scenario: profile for another command refuses

- **Given** the only profile matches command `dewey-distill`
- **When** the gate runs
- **Then** it refuses with reason `supervisor command profile unavailable`

#### Scenario: repo-local config never widens

- **Given** a repository-local supervisor config with a matching profile
- **And** no global config
- **When** the gate runs
- **Then** it refuses

### Requirement: Adoption Documentation

**ID**: FR-022 (replaces "workflow template secrets, permissions and
issue labels")

Dewey MUST document everything a knowledge repository needs to run
unattended distill without reverse-engineering: the global supervisor
profile (file location and a JSON example), the clean-checkout
precondition, the CODEOWNERS / code-owner-review prerequisite for
`human-gate` PRs, and the command's flags. The documentation MUST
describe scheduling neutrally and MUST NOT name a specific dispatcher.

#### Scenario: setup is documented neutrally

- **Given** the README's weekly unattended distill section
- **When** a maintainer reads it
- **Then** it shows the supervisor config path and JSON profile
- **And** it describes running `/dewey-distill-auto` from any scheduler
  or agent runner
- **And** it names no specific dispatcher, issue label or workflow file

### Requirement: Supervisor Prerequisite Assumption

**ID**: Assumption (opencode-supervisor#51)

Unattended mode depends on the global opencode-supervisor command
profile described in FR-016, shipped in opencode-supervisor v2.9.1.
When the profile is absent or unverifiable, the run MUST NOT proceed as
`distiller`: it records a no-op artifact and stops. No notice comment is
posted. No "tightest achievable" degradation that would allow repo-local
widening is permitted.

#### Scenario: missing prerequisite is a recorded no-op

- **Given** the supervisor profile is not configured
- **When** `/dewey-distill-auto` runs
- **Then** the gate refuses, a no-op artifact is recorded, and the run
  stops without touching any PR or issue

## REMOVED Requirements

### Requirement: Issue Dedup Logic

**ID**: FR-002

**Reason**: There is no trigger issue; the template that opened and
re-triggered it is removed (FR-001).

### Requirement: Single Open Trigger Issue Invariant

**ID**: FR-003

**Reason**: The invariant concerned the removed trigger issue.

### Requirement: Template Installability Success Criterion

**ID**: SC-008

**Reason**: The workflow template, its embedded map and its scaffold
parity tests are removed (FR-001).
