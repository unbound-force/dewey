# Delta Spec: curator-agent

<!--
  Requirement numbering allocation
  ================================
  Capability: curator-agent (prefix CA)
  Allocated to curator-agent-human-gates: CA-FR-001 .. CA-FR-008
  Reserved for future curator-agent changes: CA-FR-009 .. CA-FR-099
  This change: modifies CA-FR-007 (Auto-Answer Gate Tool) in place
  again, on top of the gate-config-path-parity version, and allocates
  no new CA ids. The capability has no archived main spec yet; its
  requirements live in the pending curator-agent-human-gates delta.
  Ids MUST stay inside the allocated range.
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
`auto-question.jsonc`-format files, parsed as JSONC with `{env:VAR}`
interpolation resolved against the process environment — from every
file a plugin process could read given the gate's environment, in this
order:

1. the file named by `AUTO_QUESTION_CONFIG`, when that variable is set
   and non-empty after trimming whitespace;
2. the home-directory path `$HOME/.config/opencode/auto-question.jsonc`,
   which MUST always be checked;
3. `$OPENCODE_CONFIG_DIR/auto-question.jsonc`, when `OPENCODE_CONFIG_DIR`
   is set and non-empty after trimming whitespace; and
4. `$XDG_CONFIG_HOME/opencode/auto-question.jsonc`, which MUST be
   checked when `XDG_CONFIG_HOME` is set to a non-empty absolute path
   whose `opencode` subdirectory is not the same directory path as the
   home-directory `opencode` subdirectory.

A value of `AUTO_QUESTION_CONFIG` or `OPENCODE_CONFIG_DIR` that is `~`
or begins with `~/` MUST be expanded against `$HOME`. A candidate whose
path equals an earlier candidate's path MUST be checked once. Path
equality MUST be determined by comparing the constructed, cleaned
paths, never by evaluating symlinks. When `AUTO_QUESTION_CONFIG` and
`OPENCODE_CONFIG_DIR` are both unset or blank, the evaluation MUST
check the same files, and produce the same verdict and output, as the
gate did before this change.

The gate MUST check files 2–4 even when `AUTO_QUESTION_CONFIG` is set,
although the plugin then reads only file 1: the gate cannot observe the
plugin process's environment, so a file some plugin process may read
remains a refusal input.

Every checked file is a refusal input. The gate MUST refuse when any
checked file's effective agent list contains `curator` (after
normalization), the wildcard `*`, or an entry whose interpolation
resolves to `curator`; a clean or absent file at one path MUST NOT
mask an offending or unverifiable file at another. A refusal MUST name
the first offending or unverifiable file in the order above.

**Effective agents.** The effective agent list and default-applied flag
reported by the verdict MUST be the ones the plugin computes in the
gate's environment: when `AUTO_QUESTION_CONFIG` is set, that file's
`agents`; otherwise the `OPENCODE_CONFIG_DIR` file's `agents` when that
file exists and defines `agents`, else the home-directory file's
`agents`; and the plugin's documented default `["autonomy"]`, with the
default-applied flag set, when the deciding file or its `agents` key is
absent. The XDG file MUST NOT contribute to the reported list.

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
when `XDG_CONFIG_HOME`, `OPENCODE_CONFIG_DIR` or `AUTO_QUESTION_CONFIG`
is set; an `XDG_CONFIG_HOME` that is set but relative; an
`OPENCODE_CONFIG_DIR` or `AUTO_QUESTION_CONFIG` value that is relative
after `~` expansion, because the plugin resolves it against a working
directory the gate cannot observe; an `AUTO_QUESTION_CONFIG` file that
does not exist, because the plugin treats a missing explicit file as
an error;
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
same environment (design assumption A4). The same holds for
`OPENCODE_CONFIG_DIR` and `AUTO_QUESTION_CONFIG`: a plugin process
whose values name files the gate's environment does not name is not
checked; checking files 2–4 regardless of `AUTO_QUESTION_CONFIG`
narrows, but does not close, that gap.

The verdict output MUST name every config file it checked, on both
pass and refusal, in the order above: the first checked file as the
config file, each further checked file as an additional checked
path. All
config-supplied values in the verdict MUST be sanitized against
control-character injection. The refusal output MUST name the
offending configuration file, the reason, and the corrective action.

Previously: the evaluation checked only the home-directory file and,
when it differed, the XDG file, with effective agents from the
home-directory file. A config directory named by `OPENCODE_CONFIG_DIR`
or a file named by `AUTO_QUESTION_CONFIG` — both read by the plugin
since opencode-auto-question 0.3.0 — went unchecked, so either could
list `curator` or `*` while the gate passed.

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

- **GIVEN** `AUTO_QUESTION_CONFIG` and `OPENCODE_CONFIG_DIR` unset
- **AND** parseable plugin configs at the home-directory path and,
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

- **GIVEN** `AUTO_QUESTION_CONFIG` and `OPENCODE_CONFIG_DIR` unset
- **AND** `XDG_CONFIG_HOME` unset or empty, or set to an absolute
  path whose `opencode` directory equals `$HOME/.config/opencode`
- **WHEN** `dewey gate auto-answer` runs
- **THEN** the evaluation MUST check exactly one `auto-question.jsonc`
- **AND** the verdict MUST name exactly that file
- **AND** the verdict and output shape MUST match the gate's behavior
  before this change

#### Scenario: Config-directory file lists curator

- **GIVEN** `OPENCODE_CONFIG_DIR` set to an absolute directory whose
  `auto-question.jsonc` `agents` list includes `curator` or `*`
- **AND** a clean home-directory config
- **WHEN** `dewey gate auto-answer` runs
- **THEN** it MUST exit non-zero
- **AND** its refusal MUST name the config-directory file

#### Scenario: AUTO_QUESTION_CONFIG file lists curator

- **GIVEN** `AUTO_QUESTION_CONFIG` naming an existing file whose
  `agents` list includes `curator` or `*`
- **AND** a clean home-directory config
- **WHEN** `dewey gate auto-answer` runs
- **THEN** it MUST exit non-zero
- **AND** its refusal MUST name the `AUTO_QUESTION_CONFIG` file

#### Scenario: AUTO_QUESTION_CONFIG does not mask the home-directory file

- **GIVEN** `AUTO_QUESTION_CONFIG` naming a clean, existing file
- **AND** a home-directory config whose `agents` list includes
  `curator`
- **WHEN** `dewey gate auto-answer` runs
- **THEN** it MUST exit non-zero
- **AND** its refusal MUST name the home-directory file

#### Scenario: Effective agents follow the plugin's merge

- **GIVEN** clean configs at the home-directory path and in
  `OPENCODE_CONFIG_DIR`, both defining `agents`, with different lists
- **WHEN** `dewey gate auto-answer` runs
- **THEN** it MUST exit zero
- **AND** the reported effective agent list MUST be the
  config-directory file's list
- **AND** when the config-directory file has no `agents` key, the
  reported list MUST be the home-directory file's list

#### Scenario: AUTO_QUESTION_CONFIG decides the effective agents

- **GIVEN** `AUTO_QUESTION_CONFIG` naming a clean file
- **AND** clean configs at the other checked paths with different
  lists
- **WHEN** `dewey gate auto-answer` runs
- **THEN** it MUST exit zero
- **AND** the reported effective agent list MUST be the
  `AUTO_QUESTION_CONFIG` file's list, or the default when it has no
  `agents` key
- **AND** the verdict MUST name the `AUTO_QUESTION_CONFIG` file first
  and every other checked file after it

#### Scenario: New source unverifiable

- **GIVEN** `AUTO_QUESTION_CONFIG` or `OPENCODE_CONFIG_DIR` set to a
  value that is relative after `~` expansion, or
  `AUTO_QUESTION_CONFIG` naming a file that does not exist
- **WHEN** `dewey gate auto-answer` runs
- **THEN** it MUST exit non-zero, naming the variable or file

#### Scenario: Effective agents come from the home-directory file

- **GIVEN** `AUTO_QUESTION_CONFIG` and `OPENCODE_CONFIG_DIR` unset
- **AND** no `auto-question.jsonc` under `$HOME/.config/opencode`
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
