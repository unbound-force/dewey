## Context

`dewey gate auto-answer` refuses when the opencode-auto-question plugin
is configured to auto-answer the `curator` agent's questions. After
`gate-config-path-parity` (#191) it checks
`$HOME/.config/opencode/auto-question.jsonc` always (authoritative for
the reported agent list) plus `$XDG_CONFIG_HOME/opencode/auto-question.jsonc`
when that directory differs; every checked file is a refusal input.

The plugin's loader (opencode-auto-question 0.3.0+, `src/config.ts`,
`configSources()` and `loadConfig()`) now reads, lowest precedence
first:

1. When `AUTO_QUESTION_CONFIG` is set and non-empty after trimming:
   that one file, `~`-expanded and `path.resolve`d against the plugin
   process's working directory, marked required. Nothing else is read.
   A missing file throws `ConfigError`; the plugin logs "config load
   failed — plugin inert" and registers no hooks.
2. Otherwise: `join(homedir(), ".config", "opencode", "auto-question.jsonc")`
   (optional), then — when `OPENCODE_CONFIG_DIR` is set and non-empty
   after trimming — `resolve(expandHome(dir), "auto-question.jsonc")`
   (optional), skipped when it equals the home-directory path. Each
   file is deep-merged over the last; arrays replace, so the last file
   that defines `agents` decides the agent list.

After merging, the plugin drops every human-gated agent (always
including `curator`) from `agents`. That is the plugin's own guarantee;
the gate is the independent one, and today it does not see sources 1
or 2b.

Relevant structure in `gate.go`: `resolvePluginConfigDirs(lookup)`
(home + optional XDG directory), the candidate loop in
`evaluateAutoAnswerGate`, `evaluateGateCandidate` →
`resolvePluginAgents` (absent file → default, directory or stat error
→ refusal), `agentListRefusal`, and `gateVerdict{ConfigFile,
AdditionalConfigFiles, EffectiveAgents, DefaultApplied, ...}`.

## Goals / Non-Goals

### Goals

- Check every file any plugin process could read given the gate's
  environment: `AUTO_QUESTION_CONFIG`, the home-directory file, the
  `OPENCODE_CONFIG_DIR` file, and the XDG file.
- Refuse on `curator`, `*`, or an unverifiable file at any of them.
- Report the effective agent list the plugin would compute.
- Byte-identical behavior and output when neither new variable is set;
  the existing gate suite passes unmodified.

### Non-Goals

- Changing the advisory registration scan (D7).
- Closing the environment-divergence residual (A4).
- Modifying the plugin (external repository).

## Decisions

### D1 Candidate set: the union, in a fixed order

Candidates, in evaluation and reporting order:

1. `$AUTO_QUESTION_CONFIG`, when set and non-empty after trimming.
2. `$HOME/.config/opencode/auto-question.jsonc` — always, as today.
3. `$OPENCODE_CONFIG_DIR/auto-question.jsonc`, when the variable is set
   and non-empty after trimming.
4. `$XDG_CONFIG_HOME/opencode/auto-question.jsonc`, under today's
   conditions.

A candidate whose cleaned path equals an earlier candidate's is
dropped (lexical comparison, no symlink evaluation — as D3 of
`gate-config-path-parity`, aliasing can only over-check). When neither
new variable is set the list is exactly today's, so the verdict is
byte-identical.

The first candidate is `ConfigFile`; the rest are
`AdditionalConfigFiles`. A refusal names the first offending or
unverifiable candidate in this order.

### D2 Over-check when `AUTO_QUESTION_CONFIG` is set

The plugin ignores files 2–4 when `AUTO_QUESTION_CONFIG` is set. The
gate still checks them. The gate and the plugin run as different
processes (A4); a user can launch opencode with a different
environment from the one `/dewey-distill` runs the gate in (gh-hud,
for example, sets `OPENCODE_CONFIG_DIR` only for its own runs).
Checking the union means a file some plugin process may read stays a
refusal input. The cost is a refusal for a file a particular plugin
process would ignore, which the user fixes by cleaning that file —
the same trade-off #191 made for the XDG file.

### D3 Effective agents follow the plugin's merge rule

`EffectiveAgents` and `DefaultApplied` are what the plugin would
compute in the gate's environment:

- `AUTO_QUESTION_CONFIG` set: that file's `agents`, or the default when
  it has no `agents` key.
- Otherwise: the `OPENCODE_CONFIG_DIR` file's `agents` when that file
  exists and defines `agents`; else the home-directory file's; else
  the default `["autonomy"]` with `DefaultApplied`.

The XDG file never contributes (the plugin never reads it), as today.
These fields are reporting only; refusal remains a union over every
candidate (D1).

### D4 Path resolution and fail-closed refusals

Both variables are trimmed; a blank value counts as unset (the
plugin's `?.trim()` truthiness check). A value of `~` or one starting
with `~/` is expanded against the resolved `$HOME` (already required to
be absolute — D7 of `gate-config-path-parity`). After expansion:

- A relative value refuses: "`AUTO_QUESTION_CONFIG` (or
  `OPENCODE_CONFIG_DIR`) `<value>` is not an absolute path". The plugin
  resolves it against its own working directory, which the gate cannot
  observe — the same reasoning that makes a relative `HOME` or
  `XDG_CONFIG_HOME` refuse.
- An `AUTO_QUESTION_CONFIG` file that does not exist refuses, naming
  the file, with the fix "point AUTO_QUESTION_CONFIG at an existing
  plugin config or unset it". The plugin treats a missing explicit file
  as an error; the gate does not guess what an inert or differently
  configured plugin does.
- A missing `OPENCODE_CONFIG_DIR` file is absent (optional in the
  plugin), handled by the existing absent-file path.
- Every other per-file condition (stat failure, directory, unreadable,
  unparseable, malformed `agents`, unset or partial `{env:VAR}`) is
  handled by the existing `resolvePluginAgents` path for every
  candidate.

### D5 Test seams unchanged

The injected `configDir` still collapses evaluation to that one
directory with no environment resolution, so every existing
single-path test keeps its meaning. New behavior is tested through
`lookupEnv` with `HOME`, `XDG_CONFIG_HOME`, `OPENCODE_CONFIG_DIR` and
`AUTO_QUESTION_CONFIG` mapped onto `t.TempDir()` trees.

### D6 Structure

Candidate construction moves into a helper that returns the ordered,
de-duplicated candidates with their role (explicit, home, config
directory, XDG), keeping `evaluateAutoAnswerGate` focused (CS-010).
The role drives D3's effective-agent selection and D4's
missing-explicit-file refusal. `resolvePluginConfigDirs` keeps its
signature and behavior.

### D7 Registration scan unchanged

`OPENCODE_CONFIG_DIR` can also hold `opencode.json(c)` and a
`plugins/` directory, so the advisory registration scan could look
there too. It never changes the verdict; widening it only changes
advisory output, so it stays out of scope as in D6 of
`gate-config-path-parity`. Possible follow-up.

## Risks / Trade-offs

1. **Over-check refusals (D2).** A user who sets
   `AUTO_QUESTION_CONFIG` for a clean config but keeps `curator` in the
   home-directory file is refused although that plugin process would
   not read it. Accepted: the gate cannot see the plugin's
   environment, and the fix is a one-line config edit.
2. **Plugin loader drift.** A future plugin release could add or
   reorder sources again. The plugin's AGENTS.md records that Dewey's
   gate mirrors its config sources and asks for both to change
   together; A1 names the version this change mirrors.
3. **Residual environment divergence (A4).** A plugin process whose
   `AUTO_QUESTION_CONFIG` or `OPENCODE_CONFIG_DIR` points somewhere the
   gate's environment does not name is unchecked. Same residual class
   as `HOME`/`XDG_CONFIG_HOME`; documented in the spec.

## Constitution Alignment

See proposal.md: Composability First PASS, Autonomous Collaboration
PASS, Observable Quality PASS, Testability PASS. The design adds no
dependency, keeps the verdict-plus-exit-code contract, names every
checked file, and is fully covered through the existing seams.

## Assumptions

### A1 The plugin loader (verified against opencode-auto-question 0.3.2)

`configSources(env)` returns `[{AUTO_QUESTION_CONFIG, required}]` when
that variable is set and non-blank, else the home-directory file then
the `OPENCODE_CONFIG_DIR` file (both optional, the latter skipped when
it equals the former). `loadConfig` deep-merges them in order with
arrays replacing; a missing required file throws and the plugin goes
inert.

### A2 `homedir()` is `$HOME` when set

Unchanged from `gate-config-path-parity` A2; also used for `~`
expansion here.

### A3 Existing tests encode the contract

Every existing gate test sets neither new variable, so they pin the
unchanged default (D1) and must pass unmodified.

### A4 The gate and plugin processes share one environment

Unchanged, and extended to the two new variables: the gate can only
check what its own environment names. D2's over-check narrows, but
does not close, the gap.
