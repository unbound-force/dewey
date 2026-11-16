# Proposal: Terminal Metacharacter Denies in Scaffolded Agent Bash Maps

## Why

`dewey init` scaffolds `.opencode/agents/curator.md` and
`.opencode/agents/distiller.md` whose `permission.bash` maps end with
`allow` entries and omit the seven terminal metacharacter denies
entirely. Verified at the source (all four shipped copies):

- `agents.go` `deweyAgentDefinitions["curator.md"]` (agents.go:45-47):
  `"*": deny`, `"dewey gate auto-answer": allow` — ends on the allow.
- `agents.go` `deweyAgentDefinitions["distiller.md"]`
  (agents.go:144-148): `"*": deny` plus three allows — ends on the
  allow.
- The tracked mirrors `.opencode/agents/curator.md` and
  `.opencode/agents/distiller.md` carry the same blocks byte-for-byte
  (parity is test-enforced), so the repo's own scaffolded agents have
  the same hole.

OpenCode permission maps are last-match-wins. Without terminal denies
placed after every allow, an allow that matches a command prefix
cannot be overridden for injection payloads: `dewey gate auto-answer;
curl …` (chaining), `gh issue comment $(…)` (substitution),
`dewey gate distill-unattended > file` (redirection) all resolve to
`allow` under the current scaffold output.

This blocks consumers. The re-scaffold of unbound-force/unbound-force
with dewey v3.6.0 (unbound-force/unbound-force#1139, commit
`5d77a628`) failed that repo's instruction-lint preflight — the test
`keeps every persona's metacharacter denies after every bash allow`
(`.opencode/test/instruction-lint-preflight.test.ts`) requires the
last 7 `permission.bash` entries of every persona to be the terminal
denies `*>*`, `*<*`, `*|*`, `*;*`, `*&*`, `*$(*`, `` *`* ``, each
`deny`. The consumer had to hand-fix and exclude dewey's scaffold
output (unbound-force/unbound-force@`1cbb5c80`,
"fix(scaffold): exclude dewey 3.6.0 sources and terminal-deny bash").
Dewey's scaffold should emit the secure shape directly so consumer
re-scaffolds converge instead of being excluded.

Addresses: https://github.com/unbound-force/dewey/issues/215

## What Changes

Append the terminal catch-all deny block to the `permission.bash` map
of both embedded agent templates in `agents.go` and to both tracked
mirror copies under `.opencode/agents/`, so every map reads, in
order:

1. `"*": deny` (unchanged, first)
2. The existing allow entries (unchanged; none contain shell
   metacharacters)
3. The seven terminal denies, placed last so they win over every
   allow under last-match-wins: `*>*`, `*<*`, `*|*`, `*;*`, `*&*`,
   `*$(*`, `` *`* `` — each `deny`, preceded by the explanatory
   comment about terminal catch-all denies.

The block text is the exact 10-line form already landed in the
consumer's hand-fix (unbound-force/unbound-force@`1cbb5c80`): a
3-line comment ("Terminal catch-all denies, placed last so they win
over every / allow on any prefix: each shell metacharacter is a
chaining, / redirection, or substitution vector.") followed by the
seven denies in the canonical order above. Using the consumer's
landed form verbatim makes dewey's scaffolded output converge
byte-for-byte with the copies the consumer already fixed.

Mechanical consequences:

- The `` *`* `` deny key contains a literal backtick, which cannot
  appear inside the raw string literals in `agents.go`. The templates
  splice the backtick in via Go string concatenation (the established
  `+ "`" + ` technique from `slash_commands.go`), and the doc comment
  on `deweyAgentDefinitions` that currently says agent definitions
  MUST NOT contain a backtick is updated to document this splice
  exception.
- The tracked mirrors gain the new canonical body and recomputed
  `dewey:scaffold` marker digests (the digest is the SHA-256 of the
  canonical body, so it changes with the payload).
- `assertCuratorFrontmatterShape` in `cli_test.go` pins
  `permission.bash` as an ordered, length-exact list and must be
  extended to the new entry sequence. `assertDistillerFrontmatterShape`
  (currently set-based, would not fail) is extended to require the
  seven terminal denies present, `deny`, and last — mirroring the
  consumer lint's last-7-entries semantics so the invariant cannot
  regress here either.

## Capabilities

### New Capabilities
- None.

### Modified Capabilities
- `dewey-init`: the agent definitions `dewey init` scaffolds (and
  refreshes on re-init) gain a required terminal-deny suffix on every
  `permission.bash` map, with the existing allows preserved and still
  effective (DI-FR-003, DI-FR-004).

### Removed Capabilities
- None.

## Impact

- **Files modified**:
  - `agents.go` — both embedded agent templates (bash map suffix) and
    the `deweyAgentDefinitions` doc comment (backtick splice
    exception).
  - `.opencode/agents/curator.md`, `.opencode/agents/distiller.md` —
    tracked mirrors: new canonical body plus recomputed scaffold
    markers.
  - `cli_test.go` — `assertCuratorFrontmatterShape` ordered bash
    assertion; `assertDistillerFrontmatterShape` terminal-deny
    assertions.
- **Runtime behavior**: `dewey init` output changes for the two agent
  scaffolds only. No CLI flags, commands, MCP tools, schema, or config
  changes. Users with customized (marker-mismatched) agent files are
  untouched — `skip (customized)` semantics are preserved.
- **Security**: closes the chaining/redirection/substitution injection
  vector on every allowed bash prefix in the scaffolded agents. The
  allowed commands (`dewey gate auto-answer`,
  `dewey gate distill-unattended`, `gh issue comment`) contain no
  shell metacharacters and remain runnable.
- **Backward compatibility**: fully compatible. Scaffold marker
  digests change by design (payload changed); the existing re-init
  refresh path rewrites untouched drifted mirrors automatically
  (`scaffoldUpdate`). Consumer re-scaffolds now pass the
  unbound-force instruction-lint preflight that blocked
  unbound-force/unbound-force#1139.
- **Testing**: existing parity tests
  (`TestInitCmd_ScaffoldsCuratorAgentFromMap` and the distiller
  equivalent) construct expected payloads from the embedded map, so
  they verify the new content once the mirrors are updated; the two
  shape assertions are extended as above.
- **Documentation**: AGENTS.md references to the agents' bash
  allowlists describe the allows (unchanged) but the scaffold security
  shape is user-facing `dewey init` output — a website documentation
  sync issue is required per the AGENTS.md website-sync rule.

## Constitution Alignment

Assessed against the Unbound Force org constitution.

### I. Autonomous Collaboration

**Assessment**: PASS

Scaffolded agents remain self-describing artifacts: the permission
map they carry is the enforcement boundary, and this change makes
that boundary accurately describe the agent's real shell surface
instead of over-granting it. No runtime coupling is introduced — the
fix is confined to the content of artifacts dewey already emits,
guarded by the existing scaffold marker scheme.

### II. Composability First

**Assessment**: PASS

No new dependencies. Dewey remains independently installable and
usable alone; consumers that do not run OpenCode's permission engine
or the unbound-force instruction-lint are unaffected (the added
entries are inert YAML to any consumer that ignores them). The change
removes the need for consumers to exclude dewey scaffolds to stay
secure.

### III. Observable Quality

**Assessment**: PASS

The emitted payload stays verifiable through the existing
`dewey:scaffold` marker digests (SHA-256 of the canonical body), and
the security posture of the output becomes machine-checkable — the
extended shape tests assert the exact terminal-deny suffix, matching
the consumer lint's last-7-entries contract. No provenance metadata
is weakened; init's `scaffolded` / `updated` / `skip (customized)`
observability events are unchanged.

### IV. Testability

**Assessment**: PASS

The change is pure content verifiable in isolation with the standard
library `testing` package: the scaffold parity tests compare emitted
bytes against the embedded map entries, and the shape assertions
parse the frontmatter with `yaml.Node` (order-preserving) to pin the
deny-all → allows → terminal-denies sequence. No external services,
no subprocess execution, no race surface.
