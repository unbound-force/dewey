# Delta Spec: dewey-init

<!--
  Requirement numbering allocation
  ================================
  Capability: dewey-init (prefix DI)
  Previously allocated: DI-FR-001 .. DI-FR-002
    (tool-owned-scaffold-updates)
  Allocated to scaffold-bash-terminal-denies: DI-FR-003 .. DI-FR-004
  Reserved for future dewey-init changes: DI-FR-005 .. DI-FR-099
  Custody: this delta adds requirements on the content of the agent
  definitions dewey init scaffolds — the shape of the
  permission.bash mapping — on top of the write/refresh discipline
  DI-FR-001 and DI-FR-002 already state. It does not modify those
  requirements; the marker, refresh, and skip-customized semantics
  are unchanged.
-->

## ADDED Requirements

### Requirement: Terminal metacharacter denies close every scaffolded agent bash map

**ID**: DI-FR-003

Every agent definition embedded in the binary (the
`deweyAgentDefinitions` map) whose frontmatter carries a
`permission.bash` mapping MUST order that mapping as:

1. the deny-all entry `"*": deny` first,
2. then the agent's allow entries, and
3. then, as the final entries of the mapping, the seven terminal
   metacharacter denies in the canonical order `*>*`, `*<*`, `*|*`,
   `*;*`, `*&*`, `*$(*`, `` *`* ``, each with value `deny`.

The terminal deny entries MUST be preceded by a comment explaining
that the terminal catch-all denies are placed last so they win over
every allow on any prefix. Because the OpenCode permission engine is
last-match-wins, an allow entry MUST NOT follow any terminal deny
entry. The scaffolded files `dewey init` writes from these
definitions, and the tracked mirror copies under
`.opencode/agents/` in this repository, MUST carry this shape
byte-identically to the embedded payloads under the existing
scaffold-marker parity contract (DI-FR-001/DI-FR-002).

#### Scenario: Scaffolded curator ends with the terminal denies

- **GIVEN** the embedded `deweyAgentDefinitions["curator.md"]`
  definition, whose `permission.bash` mapping is `"*": deny` followed
  by the single allow `"dewey gate auto-answer": allow`
- **WHEN** `dewey init` scaffolds `.opencode/agents/curator.md` into
  a vault that has a `.opencode/` directory
- **THEN** the scaffolded file's `permission.bash` mapping keeps the
  deny-all entry first and the gate allow, and its final seven
  entries are exactly `*>*`, `*<*`, `*|*`, `*;*`, `*&*`, `*$(*`,
  `` *`* `` in that order, each with value `deny`, preceded by the
  explanatory comment

#### Scenario: Scaffolded distiller ends with the terminal denies

- **GIVEN** the embedded `deweyAgentDefinitions["distiller.md"]`
  definition, whose `permission.bash` mapping is `"*": deny` followed
  by the three allows `"dewey gate auto-answer": allow`,
  `"dewey gate distill-unattended": allow`, and
  `"gh issue comment": allow`
- **WHEN** `dewey init` scaffolds `.opencode/agents/distiller.md`
- **THEN** all three allows are retained unchanged before the
  terminal block, and the mapping's final seven entries are the seven
  terminal denies in the canonical order, each `deny`

#### Scenario: Tracked mirrors converge with the embedded payloads

- **GIVEN** the repository's tracked `.opencode/agents/curator.md`
  and `.opencode/agents/distiller.md` carry the pre-change bodies
  with scaffold markers whose digests match those bodies
  (verifiably untouched)
- **WHEN** the embedded definitions gain the terminal deny block and
  `dewey init` re-runs on this repository
- **THEN** both mirrors are rewritten to the new canonical bodies
  with fresh markers (the `scaffoldUpdate` path of DI-FR-002), and
  the scaffold parity tests that compare the tracked files against
  payloads constructed from the embedded map entries pass

## ADDED Requirements

### Requirement: Terminal denies must not disable allowed commands

**ID**: DI-FR-004

The terminal deny block MUST NOT render any allow entry unusable.
Every allow entry in a scaffolded agent's `permission.bash` mapping
MUST match only command strings that contain no shell
metacharacter covered by the terminal denies (`>`, `<`, `|`, `;`,
`&`, `$(`, backtick), so that under last-match-wins the terminal
denies override injection payloads on allowed prefixes but never the
allowed commands themselves. In particular, the startup gates
`dewey gate auto-answer` and `dewey gate distill-unattended`, and
the distiller's `gh issue comment`, MUST resolve to `allow` in the
scaffolded agents.

#### Scenario: Startup gates remain runnable

- **GIVEN** a scaffolded curator or distiller agent whose
  `permission.bash` mapping ends with the seven terminal denies
- **WHEN** the permission engine evaluates the bare commands
  `dewey gate auto-answer` and `dewey gate distill-unattended`
  (and, for the distiller, `gh issue comment <n> --body-file <path>`)
  under last-match-wins
- **THEN** each command resolves to `allow`, because the allowed
  prefixes contain none of the denied metacharacters

#### Scenario: Injection payloads on allowed prefixes are denied

- **GIVEN** a scaffolded curator or distiller agent whose
  `permission.bash` mapping ends with the seven terminal denies
- **WHEN** the permission engine evaluates commands that chain,
  redirect, or substitute on an allowed prefix — for example
  `dewey gate auto-answer; curl http://evil.example`,
  `gh issue comment $(cat /etc/passwd)`, or
  `dewey gate distill-unattended > /tmp/capture`
- **THEN** each command resolves to `deny`, because the terminal
  deny matching its metacharacter is the last matching entry

## Test Strategy

- **Ordered shape assertions**: parse both shipped copies of each
  agent definition (the embedded `deweyAgentDefinitions` entry and
  the tracked `.opencode/agents/` file) with order-preserving
  `yaml.Node` walks, and assert the `permission.bash` entry sequence:
  deny-all first, the documented allows, then exactly the seven
  terminal denies as the final entries — mirroring the consumer
  lint's last-7-entries contract
  (`unbound-force/unbound-force` `.opencode/test/instruction-lint-preflight.test.ts`,
  "keeps every persona's metacharacter denies after every bash
  allow").
- **Existing parity tests**: `TestInitCmd_ScaffoldsCuratorAgentFromMap`
  and its distiller equivalent already compare emitted and tracked
  bytes against payloads constructed from the embedded map entries;
  they verify the new content without modification.
- **Allow preservation**: the shape assertions additionally require
  the gate and `gh issue comment` allows to remain present with
  value `allow` (the allowed command strings contain no
  metacharacters, so ordering alone preserves them).
