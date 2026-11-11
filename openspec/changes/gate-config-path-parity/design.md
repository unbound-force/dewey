## Context

`dewey gate auto-answer` (change `curator-agent-human-gates`, #152) is
the deterministic startup gate `/dewey-distill` runs before any
pipeline stage. It refuses when the opencode-auto-question plugin is
configured to auto-answer questions for the `curator` agent.

The gate currently resolves exactly one plugin config directory,
XDG-first (`gate.go`, `resolveOpenCodeConfigDir`):
`$XDG_CONFIG_HOME/opencode` when `XDG_CONFIG_HOME` is set and
absolute, else `$HOME/.config/opencode`. The plugin, however, always
loads `join(homedir(), ".config", "opencode", "auto-question.jsonc")`
— verified in the plugin's `src/config.ts`, whose `defaultConfigPath()`
never consults `XDG_CONFIG_HOME`. When `XDG_CONFIG_HOME` points
anywhere other than `~/.config`, the gate verifies a file the plugin
never reads while the plugin's real config goes unchecked: the gate
can PASS while the plugin's config lists `curator` or `*`, and the
human gates get auto-answered. The gate fails open (#191; found in a
post-merge review of #187).

Relevant existing structure in `gate.go`:

- `gateOptions{configDir, workDir, lookupEnv}` — the injection seams.
  An injected `configDir` bypasses directory resolution entirely.
- `evaluateAutoAnswerGate(opts)` — resolves the config dir, scans
  plugin registration (advisory), resolves the plugin agents for one
  file, refuses on `curator`/`*`/unverifiable, verifies the curator
  definition.
- `resolvePluginAgents` / `readPluginAgents` — per-file evaluation
  (stat, JSONC parse, `agents` extraction, `{env:VAR}` interpolation,
  trim + case-fold). Already path-parameterized and reusable as-is.
- `gateVerdict{ConfigFile, EffectiveAgents, DefaultApplied, ...}` —
  single-file verdict; `renderGateVerdict` prints `Config file:` once.
- `scanPluginRegistration(workDir, configDir)` — advisory scan of
  project + one global config directory.

## Goals / Non-Goals

### Goals

- The gate verifies every config path the plugin could read: the
  home-directory path always, plus the XDG path when it differs.
- Refuse when any checked path lists `curator` or `*`, or cannot be
  verified; no clean file masks an offending or unverifiable one.
- The verdict names every file checked, on pass and refusal.
- Existing single-path behavior and output stay byte-identical when
  `XDG_CONFIG_HOME` is unset, empty, or resolves to the same directory
  as `$HOME/.config` — every existing `gate_test.go` test passes
  unmodified.
- The #191 acceptance matrix is covered by unit tests.

### Non-Goals

- Changing the opencode-auto-question plugin to honor
  `XDG_CONFIG_HOME` (external repository). The gate must match the
  plugin as it behaves today.
- Expanding the advisory plugin-registration scan to multiple
  directories. It never changes the verdict; the issue targets the
  plugin config path. Recorded in D6 as a possible future
  improvement.
- Renumbering or splitting CA-FR-007. The fix rewrites that
  requirement's configuration-location contract in place.
- Any change to `/dewey-distill`, the marker grammar, the agent
  definition, or the curator capability boundary.

## Decisions

### D1 Candidate set: home directory always, XDG when lexically different

Replace `resolveOpenCodeConfigDir` with a resolution that returns the
ordered pair `(homeDir, xdgDir)`:

- `homeDir = filepath.Join(HOME, ".config", "opencode")`. `HOME` MUST
  be set, non-empty, and absolute, else resolution fails (refusal).
- `xdgDir = filepath.Join(XDG_CONFIG_HOME, "opencode")` only when
  `XDG_CONFIG_HOME` is set, non-empty, absolute, and `!= homeDir`
  (lexical comparison of the joined directory paths). A relative
  `XDG_CONFIG_HOME` fails resolution, as today.

The candidate file list is `[homeDir/auto-question.jsonc]`, plus
`[xdgDir/auto-question.jsonc]` when `xdgDir` is non-empty — home
directory first, deterministically. At most two small files are read
sequentially; no concurrency is introduced.

Rationale: the plugin reads the home-directory path unconditionally,
so it can never be skipped. The XDG path stays in the set (rather than
being dropped as "wrong") because the gate's contract is to check
every path the plugin *could* read — the conservative, fail-closed
reading — and #191's acceptance matrix requires refusal in both
directions.

### D2 Authority split: effective agents from the home file, refusal is a union

The plugin loads only the home-directory file, so the reported
`EffectiveAgents` and `DefaultApplied` MUST come from that file (or
its documented default when absent). Every checked file — including
the XDG file — is a refusal input: an offending entry, an unparseable
body, or an unverifiable stat/read at any path refuses the gate.

Evaluation reads every candidate before deciding (there are at most
two), then refuses on the first offending or unverifiable file in the
fixed home-then-XDG order, so the refusal reason and fix name exactly
one deterministic file. Reading all candidates first keeps the
verdict's "every file it checked" claim literally true and keeps the
reason deterministic.

### D3 Lexical comparison for path coincidence (no symlink evaluation)

Coincidence is decided by comparing the constructed directory paths as
strings. `filepath.EvalSymlinks` was rejected: it turns resolution
into a filesystem-dependent probe (nondeterministic across machines),
and a symlink retargeted between the comparison and the read would
reintroduce a verify-one-file-read-another race. Lexical comparison
can only over-check — when `$HOME/.config` is a symlink to the XDG
directory (or a case-insensitive filesystem aliases the two), the same
physical file is read twice with identical results, which is harmless
and fails closed. It can never treat two different files as one.

### D4 Verdict shape: keep `ConfigFile`, add `AdditionalConfigFiles`

`gateVerdict` keeps `ConfigFile string` — now defined as the
authoritative home-directory file (or the injected `configDir` path) —
and gains `AdditionalConfigFiles []string` for the remaining checked
paths, in evaluation order. `renderGateVerdict` keeps the existing
`Config file: <path>` line and adds one `Also checked: <path>` line
per additional file, immediately after it, on both PASS and REFUSED
verdicts, each passed through the existing `safeValue` sanitizer.

This preserves every existing render assertion (including
`TestRenderGateVerdict_Pass` and `assertRefusal`'s
`ConfigFile != ""` check) while making the checked-path set observable
— Constitution III. A plural `ConfigFiles` field was rejected because
it would break the existing tests' compile and assertions for no
contract gain.

### D5 Test seams: `configDir` stays single-path, multi-path goes through `lookupEnv`

An injected `gateOptions.configDir` continues to mean "evaluate
exactly this directory" — one candidate, no environment resolution,
no `Also checked` lines. That is the seam every existing test uses, so
they keep passing unmodified and continue to pin the single-path
evaluation contract (parse, refuse, default) that multi-path
evaluation reuses per file.

New multi-path tests inject `lookupEnv` with `HOME` and
`XDG_CONFIG_HOME` mapped to `t.TempDir()` directories containing real
fixture files, exercising the production resolution path end to end.
No new seam is introduced.

### D6 Registration scan directory unchanged

`scanPluginRegistration` keeps receiving exactly one global config
directory, computed with today's rule: the injected `configDir` when
present, else the XDG directory when `XDG_CONFIG_HOME` is set and
absolute, else the home-directory directory. Registration is advisory
context that never changes the verdict (CA-FR-007), so expanding its
directory set would churn warning output with no verdict effect — out
of scope for a fail-open fix. A future change may widen it now that
multiple directories are resolved.

### D7 Fail-closed tightening: `$HOME` is required even when XDG is set

Today, a set `XDG_CONFIG_HOME` makes `$HOME` irrelevant. Under the
new model the home-directory path is always checked, so an unset,
empty, or relative `$HOME` must refuse even when `XDG_CONFIG_HOME` is
set: the path the plugin actually reads is unverifiable (the plugin's
Node `homedir()` would fall back to the passwd database, which the
gate cannot see). This is a deliberate, documented tightening — the
old behavior passed on an unverified assumption.

The refusal reason for an unresolvable directory keeps the phrase
"config directory" so the existing
`TestGateAutoAnswer_ConfigDirUnresolvableRefused` assertion keeps
holding.

### D8 Structure: extract a per-file evaluation helper

`evaluateAutoAnswerGate` gains a loop over candidates; to respect
CS-010 (focused functions), the per-file work moves into a helper
that wraps the existing `resolvePluginAgents` call and returns a
small per-file result (path, agents, defaultApplied, refusal). The
config-directory resolution lives in its own function replacing
`resolveOpenCodeConfigDir`. GoDoc on `gateOptions.configDir` and
`evaluateAutoAnswerGate` is updated to state the new semantics:
injected `configDir` collapses evaluation to one path; resolution
otherwise checks the home-directory path always plus the XDG path
when it differs.

## Risks / Trade-offs

- **New refusals in exotic environments.** A user with `XDG_CONFIG_HOME`
  set but no usable `$HOME` now gets a refusal where the gate
  previously passed. This is the intended fail-closed direction; the
  refusal names the unresolvable input and the fix. Rare in practice
  (containers sometimes omit `HOME` — they now get an honest refusal
  instead of an unverified pass).
- **Verdict output change for XDG-divergent users.** When the paths
  differ, `Config file:` now names the home-directory file (what the
  plugin reads) rather than the XDG file, plus an `Also checked:` line.
  Honest and required by #191's "name every file" acceptance; bounded
  to one extra line per additional path (at most one).
- **Double-read under path aliasing.** Symlinked or case-aliased
  coincidence reads the same file twice. Harmless (identical results,
  tiny files, sequential); documented in D3.
- **Env-manipulation residual, documented not closed.** An
  env-assignment prefix on the gate's single allowed shell command
  (`HOME=<clean-dir> dewey gate auto-answer`, `XDG_CONFIG_HOME`
  unset) redirects the home-directory lookup to a clean file while
  the plugin process still reads the real home directory — a false
  PASS, because an absolute spoofed path is not covered by the
  unset/empty/relative `$HOME` refusal. The parent change
  curator-agent-human-gates (D2) documented this class of residual
  for `XDG_CONFIG_HOME` (learning
  security-20261111T020756-jay-flowers, from the #187 post-merge
  review that filed #191) and asked that it be recorded rather than
  claimed closed; under D1's candidate set the vector migrates to
  `HOME`. Recorded as a known residual in the spec delta's CA-FR-007
  and in assumption A4 below; fixing it is out of scope for this
  change.
- **Existing-test compatibility.** The design keeps `ConfigFile`, the
  `configDir` seam, and the "config directory" refusal wording
  specifically so the existing suite passes unmodified. If
  implementation drifts from these, a test breakage signals scope
  creep, not a stale test.
- **Archive ordering constraint.** The delta MODIFIES CA-FR-007, but
  `curator-agent` has no archived main spec yet — its requirements
  live in the pending `curator-agent-human-gates` delta. `openspec
  validate` reports (informational) that archiving this change before
  its parent would be refused: MODIFIED requires the target spec to
  exist. `curator-agent-human-gates` MUST archive first (creating
  `openspec/specs/curator-agent/` with CA-FR-007); this delta's
  MODIFIED then applies cleanly. Same posture as
  `serialize-supersession-writes` against the pending
  `learning-temporal-validity` delta.

## Constitution Alignment

The proposal assesses all four principles as PASS; the design notes
above carry the alignment:

- **Autonomous Collaboration (II)** — D2 keeps the gate's verdict an
  honest artifact of what the plugin will actually do, removing a
  divergence between collaborators rather than adding coupling.
- **Composability First (I)** — D1 degrades gracefully: absent files
  at every checked path still pass on the documented default; no new
  dependency.
- **Observable Quality (III)** — D4 makes the checked-path set part of
  the tool-rendered verdict on every outcome, sanitized through the
  existing pipeline.
- **Testability (IV)** — D5 keeps every new behavior reachable
  through the existing injection seams with `t.TempDir()` fixtures;
  no external services.

## Assumptions

### A1 Plugin config path is hardcoded to the home directory (verified)

The plugin's `src/config.ts` `defaultConfigPath()` returns
`join(homedir(), ".config", "opencode", "auto-question.jsonc")` with
no `XDG_CONFIG_HOME` consultation — verified by reading the plugin
source, and cited by #191 (`src/config.ts:9`).

### A2 The plugin's `homedir()` is `$HOME` when set

Node's `os.homedir()` returns `$HOME` when it is set and non-empty;
otherwise it falls back to the passwd database. The gate mirrors the
set case and refuses on the fallback case (D7) because the passwd
home is not deterministically observable from the environment.

### A3 Existing tests encode the contract, not the bug

The existing `gate_test.go` suite pins per-file evaluation and the
single-path seam. Tests that inject `configDir` do not assert
XDG-first *resolution* (that is only reachable through `lookupEnv`),
so preserving them is compatible with fixing the resolution bug.

### A4 The gate and plugin processes share one environment

The gate is executed as a subprocess of the same session that would
run the plugin, and both resolve the home directory from `HOME` in
that shared environment. The multi-path evaluation rests on this
parity: the gate cannot detect an environment that differs between
the two processes, so an env-assignment prefix that redirects the
gate's lookup (the residual recorded above and in the spec delta's
CA-FR-007) yields a false PASS. Fixing that vector — host-side
environment hygiene on the single allowed shell command — is out of
scope for this change.
