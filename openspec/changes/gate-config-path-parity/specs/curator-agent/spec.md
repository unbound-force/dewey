# Delta Spec: curator-agent

<!--
  Requirement numbering allocation
  ================================
  Capability: curator-agent (prefix CA)
  Allocated to curator-agent-human-gates: CA-FR-001 .. CA-FR-008
  Reserved for future curator-agent changes: CA-FR-009 .. CA-FR-099
  This change: modifies CA-FR-007 (Auto-Answer Gate Tool) in place and
  allocates no new CA ids. The capability has no archived main spec
  yet; its requirements live in the pending
  curator-agent-human-gates delta, which this delta modifies the same
  way serialize-supersession-writes modified the pending
  learning-temporal-validity delta. Ids MUST stay inside the allocated
  range.
-->

## ADDED Requirements

_None._

## MODIFIED Requirements

### Requirement: Auto-Answer Gate Tool

**ID**: CA-FR-007

Dewey MUST provide a `dewey gate auto-answer` CLI subcommand that
deterministically evaluates whether the `curator` agent is configured
for automatic question answering, prints a tool-rendered verdict, and
exits 0 on pass and 1 on refusal.

**Configuration paths.** The evaluation MUST read the
opencode-auto-question plugin's own configuration —
`auto-question.jsonc`, parsed as JSONC with `{env:VAR}` interpolation
resolved against the process environment — from every directory the
plugin could read it from:

- the home-directory path `$HOME/.config/opencode/auto-question.jsonc`,
  which MUST always be checked. The plugin loads
  `join(homedir(), ".config", "opencode", "auto-question.jsonc")`
  unconditionally (its `src/config.ts` never consults
  `XDG_CONFIG_HOME`), so the home-directory file is authoritative: the
  effective agent list and the default-applied flag reported by the
  verdict MUST come from this file alone (with the plugin's documented
  default `["autonomy"]` applying when the file or its `agents` key is
  absent); and
- `$XDG_CONFIG_HOME/opencode/auto-question.jsonc`, which MUST be
  checked when `XDG_CONFIG_HOME` is set to a non-empty absolute path
  whose `opencode` subdirectory is not the same directory path as the
  home-directory `opencode` subdirectory.

Path coincidence MUST be determined by comparing the constructed
directory paths. When `XDG_CONFIG_HOME` is unset, empty, or yields the
same directory path as `$HOME/.config`, the evaluation MUST check
exactly one file and MUST produce the same verdict and output shape
the gate produced before this change.

Every checked file is a refusal input. The gate MUST refuse when any
checked file's effective agent list contains `curator` (after
normalization), the wildcard `*`, or an entry whose interpolation
resolves to `curator`; a clean or absent file at one path MUST NOT
mask an offending or unverifiable file at another.

Entry matching MUST stay pinned so the gate and the plugin agree:
every entry is whitespace-trimmed and then case-folded before
comparison, and `{env:VAR}` interpolation applies only when the entire
entry is exactly the interpolation token — a partial interpolation
inside a larger entry is treated as unverifiable rather than matched.

The tool MUST additionally verify that the curator binding is
resolvable: `.opencode/agents/curator.md`, relative to the working
directory, MUST exist and its frontmatter MUST declare
`mode: subagent`. The tool MUST additionally scan project and global
OpenCode configurations and plugin discovery directories for plugin
registration and report the result as advisory context that does not
change the verdict.

The tool MUST refuse (exit 1) when any checked configuration cannot be
located, verified, read, parsed, or interpreted. That fail-closed
surface includes the pre-read steps: a home-directory config path that
cannot be resolved (`$HOME` unset, empty, or relative) — including
when `XDG_CONFIG_HOME` is set, because the path the plugin actually
reads is then unverifiable; an `XDG_CONFIG_HOME` that is set but
relative;
and a stat of any checked `auto-question.jsonc` that fails for any
reason other than file-not-found (for example a permission error on
the config directory). It also includes an `agents` value that is not
an array of strings, an entry whose environment variable is unset, and
an entry carrying a partial interpolation — in any checked file.

Known residual, documented rather than closed here: the gate runs as
a subprocess, so its environment can be redirected independently of
the plugin process's environment. An env-assignment prefix on the
gate's single allowed shell command — `HOME=<dir> dewey gate
auto-answer`, stripped by a host that matches on the bare command —
points the home-directory lookup at attacker-chosen content while
the plugin still reads the real home directory; the refusal for an
unset, empty, or relative `$HOME` does not cover an absolute spoofed
path. The parent change (curator-agent-human-gates) documented the
same class of residual for `XDG_CONFIG_HOME`; this change moves the
vector to `HOME` without closing it. The evaluation rests on the
assumption that the gate process and the plugin process observe the
same environment (design assumption A4).

The verdict output MUST name every config file it checked, on both
pass and refusal. The authoritative home-directory file MUST be named
first; each additional checked path MUST also be named. All
config-supplied values in the verdict MUST be sanitized against
control-character injection. The refusal output MUST name the
offending configuration file, the reason, and the corrective action.

Previously: the evaluation read exactly one config file, chosen
XDG-first — `$XDG_CONFIG_HOME/opencode/auto-question.jsonc` when
`XDG_CONFIG_HOME` was set, else `~/.config/opencode/auto-question.jsonc`
— so a divergent `XDG_CONFIG_HOME` made the gate verify a file the
plugin never reads while the plugin's real config went unchecked, and
a set `XDG_CONFIG_HOME` masked an unresolvable `$HOME`.

#### Scenario: Curator configured for auto-answer

- **GIVEN** a checked plugin config whose `agents` list includes
  `curator`
- **WHEN** `dewey gate auto-answer` runs
- **THEN** it MUST exit non-zero
- **AND** its output MUST name the offending config file, the
  offending entry, and the fix

#### Scenario: Wildcard configured for auto-answer

- **GIVEN** a checked plugin config whose `agents` list is `*`
- **WHEN** `dewey gate auto-answer` runs
- **THEN** it MUST exit non-zero

#### Scenario: Plugin config absent at every checked path

- **GIVEN** no `auto-question.jsonc` at any checked config path
- **WHEN** `dewey gate auto-answer` runs
- **THEN** it MUST exit zero
- **AND** its output MUST report that the documented default agent
  list applies
- **AND** its output MUST name every checked path

#### Scenario: Clean agent list at every checked path

- **GIVEN** parseable plugin configs at the home-directory path and,
  when checked, the XDG path, whose `agents` lists contain neither
  `curator` nor `*`
- **WHEN** `dewey gate auto-answer` runs
- **THEN** it MUST exit zero
- **AND** its output MUST report the effective agent list and whether
  the default applied, taken from the home-directory file
- **AND** its output MUST name every checked file

#### Scenario: Clean XDG file does not mask curator in the home-directory file

- **GIVEN** `XDG_CONFIG_HOME` set to an absolute path whose `opencode`
  directory differs from `$HOME/.config/opencode`
- **AND** a clean `auto-question.jsonc` under
  `$XDG_CONFIG_HOME/opencode`
- **AND** an `auto-question.jsonc` under `$HOME/.config/opencode` whose
  `agents` list includes `curator` or `*`
- **WHEN** `dewey gate auto-answer` runs
- **THEN** it MUST exit non-zero
- **AND** its refusal MUST name the home-directory config file

#### Scenario: Curator in the XDG file does not mask a clean home-directory file

- **GIVEN** `XDG_CONFIG_HOME` set to an absolute path whose `opencode`
  directory differs from `$HOME/.config/opencode`
- **AND** an `auto-question.jsonc` under `$XDG_CONFIG_HOME/opencode`
  whose `agents` list includes `curator` or `*`
- **AND** a clean `auto-question.jsonc` under `$HOME/.config/opencode`
- **WHEN** `dewey gate auto-answer` runs
- **THEN** it MUST exit non-zero
- **AND** its refusal MUST name the XDG config file

#### Scenario: Verdict names every checked file

- **GIVEN** configs exist at both the home-directory path and a
  divergent XDG path, clean or offending
- **WHEN** `dewey gate auto-answer` prints its verdict
- **THEN** the output MUST name the home-directory config file
- **AND** the output MUST name the XDG config file
- **AND** the same file-naming MUST appear on pass and refusal
  verdicts

#### Scenario: Coincident paths checked once

- **GIVEN** `XDG_CONFIG_HOME` unset or empty, or set to an absolute
  path whose `opencode` directory equals `$HOME/.config/opencode`
- **WHEN** `dewey gate auto-answer` runs
- **THEN** the evaluation MUST check exactly one `auto-question.jsonc`
- **AND** the verdict MUST name exactly that file
- **AND** the verdict and output shape MUST match the gate's behavior
  before this change

#### Scenario: Effective agents come from the home-directory file

- **GIVEN** no `auto-question.jsonc` under `$HOME/.config/opencode`
  and a clean, parseable one under a divergent
  `$XDG_CONFIG_HOME/opencode`
- **WHEN** `dewey gate auto-answer` runs
- **THEN** it MUST exit zero
- **AND** the reported effective agent list MUST be the documented
  default `["autonomy"]` with the default-applied flag set
- **AND** the output MUST name both checked paths

#### Scenario: Unreadable plugin configuration

- **GIVEN** a checked plugin config that exists but cannot be read
- **WHEN** `dewey gate auto-answer` runs
- **THEN** it MUST exit non-zero rather than assume auto-answering
  is off

#### Scenario: Unparseable plugin configuration

- **GIVEN** a checked plugin config that exists but is not valid JSONC
- **WHEN** `dewey gate auto-answer` runs
- **THEN** it MUST exit non-zero

#### Scenario: Malformed agent list

- **GIVEN** a checked plugin config whose `agents` value is not an
  array of strings, or whose entry references an environment variable
  that is unset
- **WHEN** `dewey gate auto-answer` runs
- **THEN** it MUST exit non-zero

#### Scenario: Padded or partially interpolated entry refused

- **GIVEN** a checked plugin config whose `agents` entry is
  `" curator "` with surrounding whitespace, or an entry that embeds
  an interpolation token inside a larger string
- **WHEN** `dewey gate auto-answer` runs
- **THEN** the trimmed, case-folded `curator` entry MUST cause a
  non-zero exit
- **AND** the partial interpolation MUST be treated as unverifiable
  and exit non-zero

#### Scenario: Configuration location unverifiable

- **GIVEN** `$HOME` is unset, empty, or relative — including when
  `XDG_CONFIG_HOME` is set — or `XDG_CONFIG_HOME` is set but relative,
  or a checked config path cannot be stat'd for a reason other than
  file-not-found
- **WHEN** `dewey gate auto-answer` runs
- **THEN** it MUST exit non-zero rather than assume auto-answering
  is off

#### Scenario: Curator definition unresolvable

- **GIVEN** `.opencode/agents/curator.md` is absent or its frontmatter
  does not declare `mode: subagent`
- **WHEN** `dewey gate auto-answer` runs
- **THEN** it MUST exit non-zero, naming the unresolved binding

#### Scenario: Registration reported as context

- **GIVEN** any evaluated configuration
- **WHEN** `dewey gate auto-answer` prints its verdict
- **THEN** the output MUST report whether the plugin was found
  registered in the scanned project and global configurations
- **AND** a scanned configuration that exists but cannot be read or
  parsed MUST produce a warning line naming it, without changing the
  verdict

## REMOVED Requirements

_None._
