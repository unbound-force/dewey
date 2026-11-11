<!-- originating_issue: 191 -->

## Why

`dewey gate auto-answer` is the deterministic startup gate that keeps
`/dewey-distill`'s two human gates human (change
`curator-agent-human-gates`, #152). It refuses when the
opencode-auto-question plugin is configured to auto-answer questions
for the `curator` agent. That guarantee currently has a hole: the gate
and the plugin read different files.

The gate resolves the plugin config from
`$XDG_CONFIG_HOME/opencode/auto-question.jsonc` whenever
`XDG_CONFIG_HOME` is set (`gate.go`, `resolveOpenCodeConfigDir`). The
plugin, however, always loads
`join(homedir(), ".config", "opencode", "auto-question.jsonc")` — its
`defaultConfigPath()` in `src/config.ts` never consults
`XDG_CONFIG_HOME`. When `XDG_CONFIG_HOME` points anywhere other than
`~/.config`, the gate checks a file the plugin never reads. The gate
can then PASS while the plugin's real config lists `curator` (or the
wildcard `*`), and the plugin auto-answers the human gates. The gate
fails open — the exact failure mode it exists to prevent.

Found in a post-merge review of #187 (which closed #152); filed as
#191, part of the #153 umbrella.

## What Changes

- **Multi-path evaluation**: `evaluateAutoAnswerGate` no longer picks
  one config directory. It evaluates every path the plugin could read:
  the home-directory path (`$HOME/.config/opencode/auto-question.jsonc`,
  always — it is the path the plugin actually loads), plus
  `$XDG_CONFIG_HOME/opencode/auto-question.jsonc` when
  `XDG_CONFIG_HOME` is set to an absolute path that differs from the
  home-directory config directory. The gate refuses when any checked
  path lists `curator` or `*`, or cannot be verified. Both files are
  read and reported, so a clean XDG file can no longer mask a dirty
  home-directory file, and vice versa.
- **Authoritative file**: the plugin loads the home-directory file
  unconditionally, so that file remains the source of the reported
  effective agent list and default-applied flag. The XDG file, when
  checked, contributes refusal decisions and verdict reporting only.
- **Fail-closed tightening**: the home-directory path must now be
  resolvable even when `XDG_CONFIG_HOME` is set. An unset or relative
  `$HOME` alongside a set `XDG_CONFIG_HOME` previously passed on the
  XDG file alone; it now refuses, because the path the plugin actually
  reads cannot be verified. A relative `XDG_CONFIG_HOME` continues to
  refuse. Path coincidence (`XDG_CONFIG_HOME` unset, or lexically
  equal to `$HOME/.config`) keeps today's single-path behavior and
  output.
- **Observable verdict**: the verdict output names every config file it
  checked. The existing `Config file:` line continues to name the
  authoritative home-directory file; each additional checked path adds
  an `Also checked:` line, on both PASS and REFUSED verdicts, sanitized
  like all other config-supplied values.
- **Tests**: acceptance tests from #191 — XDG clean + home-directory
  config listing `curator` refuses; the reverse refuses; both clean
  passes with every checked file named in the verdict — plus edge
  cases: lexically coincident paths evaluate one file, both files
  absent passes on the documented default, an unparseable or wildcard
  file at either path refuses, and an unresolvable `$HOME` refuses.
  Existing single-path tests keep passing unmodified: the injected
  `configDir` seam still collapses evaluation to that one directory.
- **Documentation**: README's `dewey gate` section gains the
  multi-path evaluation rule and the new verdict lines; AGENTS.md gets
  a Recent Changes entry; the website tracking issue
  unbound-force/website#371 is extended with the gate's path-parity
  behavior so the published docs match the tool.

## Capabilities

### New Capabilities

- None.

### Modified Capabilities

- `curator-agent`: the Auto-Answer Gate Tool requirement (CA-FR-007)
  is modified. The gate's configuration-location contract changes from
  "the XDG-or-home config directory" to "every directory the plugin
  could read: the home-directory path always, plus the XDG path when it
  differs", with the home-directory file authoritative for the
  effective agent list, refusal on any offending or unverifiable
  checked path, and every checked file named in the verdict. The
  capability has no archived main spec yet; its requirements live in
  the pending `curator-agent-human-gates` delta, which this change
  modifies in place (the same pattern `serialize-supersession-writes`
  used against the pending `learning-temporal-validity` delta).

### Removed Capabilities

- None.

## Impact

- `gate.go` — config-path candidate resolution (home directory always,
  XDG when different, lexical comparison), per-file evaluation loop,
  refusal union, verdict fields and rendering for additional checked
  files, GoDoc updates for the changed seam semantics.
- `gate_test.go` — new acceptance and edge tests over `lookupEnv`
  injection with real temporary `HOME` and `XDG_CONFIG_HOME`
  directories; existing tests unchanged.
- `README.md` — `dewey gate` section: multi-path rule, `Also checked:`
  verdict lines, tightened fail-closed description.
- `AGENTS.md` — Recent Changes entry.
- Website tracking: extend unbound-force/website#371 (comment) with the
  multi-path gate evaluation and verdict output rather than filing a
  duplicate.
- No MCP tool contract changes, no schema changes, no new module
  dependencies, no changes outside the gate's own evaluation.

**Out of scope:** changing the opencode-auto-question plugin to honor
`XDG_CONFIG_HOME` (external repository; the gate must match the plugin
as it behaves today), changing the advisory plugin-registration scan's
directory set (advisory only, never changes the verdict — recorded as
a design note), and renumbering or splitting CA-FR-007.

## Constitution Alignment

Assessed against the Dewey constitution (v1.4.0).

### I. Composability First

**Assessment**: PASS

The change adds no dependency and removes none. The gate still works
without the plugin: absent config files at every checked path apply the
documented default `["autonomy"]` and pass. Dewey remains installable
and usable without OpenCode, the plugin, or any XDG configuration; the
multi-path read is purely additional verification of the same
file-based contract.

### II. Autonomous Collaboration

**Assessment**: PASS

The gate's contract stays an artifact: a tool-rendered verdict plus an
exit code, consumed by `/dewey-distill` command prose. The change
aligns the gate's file reads with the plugin's documented load path —
it removes a divergence between two collaborators' views of the same
configuration, rather than adding runtime coupling. No shared state or
direct calls are introduced.

### III. Observable Quality

**Assessment**: PASS (improved)

Every config file the gate checked is now named in the verdict, on
both PASS and REFUSED, sanitized through the existing control-character
stripping. A refusal names the offending file, the reason, and the
fix. The verdict is machine-parseable line-oriented output with
provenance of the decision — strictly more auditable than the
single-file report it replaces, which is what made the divergence
invisible.

### IV. Testability

**Assessment**: PASS

All new logic is exercised through the existing injection seams
(`gateOptions.lookupEnv` with `HOME` and `XDG_CONFIG_HOME` mapped to
`t.TempDir()` directories; the `configDir` seam for single-path
evaluation). The #191 acceptance matrix — both refusal directions,
both-clean pass, every-file-named verdict — is covered by unit tests
with no external services, and every existing gate test keeps passing
unmodified.
