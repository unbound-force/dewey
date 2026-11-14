## Why

`dewey gate auto-answer` is the deterministic startup gate that keeps
`/dewey-distill`'s two human gates human (change
`curator-agent-human-gates`, #152). It refuses when the
opencode-auto-question plugin is configured to auto-answer questions
for the `curator` agent. Change `gate-config-path-parity` (#191) made
the gate check every config path the plugin could read *at the time*:
the home-directory file always, plus the XDG file when it differs.

The plugin has since gained two more config sources (opencode-auto-question
0.3.0, `configSources()` in `src/config.ts`):

- `$OPENCODE_CONFIG_DIR/auto-question.jsonc` — read after the global
  file and deep-merged over it whenever `OPENCODE_CONFIG_DIR` is set,
  so a tool can ship its agents, guidance and auto-question settings
  in one directory (gh-hud does this for its supervisor runs).
  Arrays replace, so an `agents` key in this file is the effective
  agent list.
- `$AUTO_QUESTION_CONFIG` — names one file that is used *alone*: the
  global file and the config-directory file are not read. A missing
  file is an error and leaves the plugin inactive.

The gate reads neither. A config directory whose `auto-question.jsonc`
lists `curator` or `*` passes the gate today, as does an
`AUTO_QUESTION_CONFIG` file that does. The plugin itself drops
human-gated agents (always including `curator`) after merging every
source, so curator questions are still not auto-answered — but the
gate exists precisely so that guarantee does not rest on the plugin
alone. It is fail-open again, for the same reason as #191: the gate
and the plugin read different files.

## What Changes

- **Two new candidate files.** `evaluateAutoAnswerGate` adds
  `$OPENCODE_CONFIG_DIR/auto-question.jsonc` when `OPENCODE_CONFIG_DIR`
  is set (non-empty after trimming) and `$AUTO_QUESTION_CONFIG` when
  that is set, to the files it checks. Every checked file remains a
  refusal input: `curator`, `*`, or an unverifiable file at any checked
  path refuses, and no clean file masks another.
- **Over-check, never under-check.** The gate keeps checking the
  home-directory file (and the XDG file when it differs) even when
  `AUTO_QUESTION_CONFIG` is set, although the plugin then ignores them:
  the gate process and the plugin process need not share one
  environment (assumption A4), so a file the plugin might read in
  another process stays a refusal input.
- **Effective agents follow the plugin's merge.** The reported
  effective agent list and default-applied flag follow the plugin's
  load rule: the `AUTO_QUESTION_CONFIG` file alone when it is set;
  otherwise the config-directory file's `agents` when it defines
  `agents`, else the home-directory file's, else the documented
  default `["autonomy"]`. The XDG file still never contributes to the
  reported list (the plugin never reads it).
- **Order and naming.** The first checked file — `Config file:` in the
  verdict — is the `AUTO_QUESTION_CONFIG` file when set, else the
  home-directory file. The rest follow as `Also checked:` lines in the
  order home, config directory, XDG, with lexically identical paths
  checked once. A refusal names the first offending or unverifiable
  file in that order.
- **Fail-closed additions.** Both variables accept `~` / `~/...` (the
  plugin expands them against the home directory). A value that is
  relative after that expansion refuses: the plugin resolves it
  against its own working directory, which the gate cannot observe. An
  `AUTO_QUESTION_CONFIG` file that does not exist refuses (the plugin
  treats it as an error), with a fix that says to point it at an
  existing file or unset it. A missing config-directory file is
  absent, as for the home-directory file.
- **Unchanged when neither variable is set.** With
  `OPENCODE_CONFIG_DIR` and `AUTO_QUESTION_CONFIG` unset or blank, the
  candidate set, verdict and output are byte-identical to today; the
  existing gate suite passes unmodified. The injected `configDir` test
  seam still collapses evaluation to that one directory.
- **Tests**: acceptance tests over `lookupEnv` injection with real
  temporary trees for each new source, both refusal directions, merge
  authority, fail-closed cases, and the unchanged default.
- **Documentation**: README `dewey gate` section, AGENTS.md Recent
  Changes entry; website tracking issue unbound-force/website#371 is
  extended with the new sources.

## Capabilities

### New Capabilities

- None.

### Modified Capabilities

- `curator-agent`: the Auto-Answer Gate Tool requirement (CA-FR-007)
  is modified in place again. Its configuration-location contract
  grows from "home-directory path always, plus the XDG path when it
  differs" to also include the `OPENCODE_CONFIG_DIR` file and the
  `AUTO_QUESTION_CONFIG` file, with the reported effective agent list
  following the plugin's merge rule. The capability has no archived
  main spec yet; its requirements live in the pending
  `curator-agent-human-gates` delta, already modified in place by
  `gate-config-path-parity`; this delta restates the requirement in
  full on top of that version.

### Removed Capabilities

- None.

## Impact

- `gate.go` — candidate resolution for the two new sources (trim,
  `~` expansion against `$HOME`, absolute-path check, lexical
  de-duplication), a missing-file refusal for `AUTO_QUESTION_CONFIG`,
  effective-agent selection by the plugin's merge rule, GoDoc.
- `gate_test.go` — new acceptance and edge tests; existing tests
  unchanged.
- `README.md` — `dewey gate` section: the four sources, ordering,
  effective-agent rule, new refusals.
- `AGENTS.md` — Recent Changes entry.
- Website tracking: extend unbound-force/website#371 (comment).
- No MCP tool contract changes, no schema changes, no new module
  dependencies, no change to the advisory registration scan.

**Out of scope:** the registration scan's directory set (advisory
only; `OPENCODE_CONFIG_DIR` can also hold `opencode.json` and
`plugins/`, recorded as a possible follow-up in design D7), closing
the environment-divergence residual (A4), and changes to the plugin.

## Constitution Alignment

Assessed against the Dewey constitution (v1.4.0).

### I. Composability First

**Assessment**: PASS

No dependency is added or removed. With neither new variable set the
gate behaves exactly as before, and it still works without the plugin:
absent files at every checked path pass on the documented default.

### II. Autonomous Collaboration

**Assessment**: PASS

The gate's contract stays a tool-rendered verdict plus an exit code.
The change aligns the gate's file reads with the plugin's documented
load order, removing a divergence between two collaborators' views of
the same configuration without adding runtime coupling.

### III. Observable Quality

**Assessment**: PASS

Every file the gate checked is named in the verdict, on pass and
refusal, sanitized as before; refusals name the offending file, the
reason and the fix. The new fail-closed refusals carry the variable
name and value that could not be verified.

### IV. Testability

**Assessment**: PASS

All new behavior is exercised through the existing `lookupEnv` seam
with `t.TempDir()` trees; no external services. The existing gate
suite is the regression guard for the unchanged default and passes
unmodified.
