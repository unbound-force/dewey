# Design: Terminal Metacharacter Denies in Scaffolded Agent Bash Maps

## Context

`dewey init` scaffolds exactly two agent definitions from the embedded
`deweyAgentDefinitions` map (agents.go) — `curator.md` and `distiller.md`
— and this repository tracks byte-identical mirrors under
`.opencode/agents/`. The other agent files in `.opencode/agents/`
(divisor-*, worker, …) belong to other tooling and are not dewey's to
manage; no slash command or workflow template carries an OpenCode
`permission.bash` map (verified — the only `permission` hits in
`slash_commands.go`/`workflow_templates.go` are unrelated prose and the
GitHub Actions `permissions:` block).

Current state of the four shipped copies (verified at the source):

| Copy | `permission.bash` entries | Ends on |
|---|---|---|
| `deweyAgentDefinitions["curator.md"]` (agents.go:45-47) | `"*": deny`, `"dewey gate auto-answer": allow` | the allow |
| `deweyAgentDefinitions["distiller.md"]` (agents.go:144-148) | `"*": deny` + 3 allows (`dewey gate auto-answer`, `dewey gate distill-unattended`, `gh issue comment`) | the allow |
| `.opencode/agents/curator.md` (tracked mirror) | same, byte-identical; marker `dev sha256:81a87a8e…` | the allow |
| `.opencode/agents/distiller.md` (tracked mirror) | same, byte-identical; marker `dev sha256:9e1c3054…` | the allow |

The OpenCode permission engine is last-match-wins over prefix globs,
and a glob allow cannot restrict a command's arguments. With every map
ending on an allow, injection payloads on allowed prefixes resolve to
`allow`: `dewey gate auto-answer; curl …` (chaining), `gh issue comment
$(…)` (substitution), `dewey gate distill-unattended > file`
(redirection).

This blocks consumers: the unbound-force/unbound-force re-scaffold
(#1139) failed that repo's instruction-lint preflight — the test
`keeps every persona's metacharacter denies after every bash allow`
(`.opencode/test/instruction-lint-preflight.test.ts`) requires the last
7 `permission.bash` entries of every persona to be the terminal denies
`*>*`, `*<*`, `*|*`, `*;*`, `*&*`, `*$(*`, `` *`* ``, each `deny`. The
consumer hand-fixed its copies and excluded dewey's scaffold output
(unbound-force/unbound-force@`1cbb5c80`). Dewey's scaffold should emit
the secure shape directly so consumer re-scaffolds converge instead of
being excluded.

The guard landscape that must move with the content:

- `assertCuratorFrontmatterShape` (cli_test.go:5705) pins
  `permission.bash` as an **ordered, length-exact** list — `wantBash`
  currently `[{"*", "deny"}, {"dewey gate auto-answer", "allow"}]`
  (cli_test.go:5891-5911).
- `assertDistillerFrontmatterShape` (cli_test.go:6201) checks the bash
  block **set-based** (`bashByKey`, cli_test.go:6318-6336): `"*"` deny
  and the two gate allows — it would not fail on the missing terminal
  denies, and it never asserts `gh issue comment`.
- Byte-parity tests construct expected payloads from the embedded map
  at test time (`TestInitCmd_ScaffoldsCuratorAgentFromMap`
  cli_test.go:5230; `TestInitCmd_FirstInitWritesVerifiableMarkers`
  cli_test.go:7823, which iterates every `deweyAgentDefinitions`
  entry), so they verify the new content once the mirrors are
  regenerated — no modification needed.
- `TestCuratorAgent_GuardsRejectDoctoredDocuments` (cli_test.go:6022)
  executes the curator shape guard's rejection paths against doctored
  fixtures; no distiller equivalent exists.

## Goals / Non-Goals

### Goals

- Every scaffolded agent's `permission.bash` map ends with the seven
  terminal metacharacter denies in the canonical order, preceded by
  the explanatory comment — byte-identical to the consumer's landed
  form (DI-FR-003).
- The existing allow entries remain effective: `dewey gate
  auto-answer`, `dewey gate distill-unattended`, and `gh issue
  comment` still resolve to `allow` (DI-FR-004).
- Embedded templates and tracked mirrors converge byte-for-byte, with
  marker digests recomputed through the existing scaffold machinery.
- The shape guards extend so the invariant cannot regress here:
  ordered curator assertion, distiller last-7 assertion mirroring the
  consumer lint's contract, with doctored-fixture cases executing the
  new rejection arms.
- Consumer re-scaffolds of unbound-force/unbound-force pass the
  instruction-lint preflight (convergence, not exclusion).

### Non-Goals

- No new agents, no changes to the allow entries, no CLI flags,
  commands, MCP tools, schema, or config changes.
- No change to the marker scheme, the classification decision table,
  or the refresh / `skip (customized)` semantics (DI-FR-001/DI-FR-002
  are untouched; this change only rides them).
- No permission-engine runtime changes in dewey itself — dewey emits
  YAML; OpenCode interprets it.
- No repair of consumer copies that are already customized (the
  `skip (customized)` path preserves them by design).
- No terminal denies for the repo's non-dewey agent files
  (divisor-*, worker, …) — they are not scaffolded by dewey.

## Decisions

### D1 The terminal-deny block: exact text, canonical order, placed after every allow

The block is the consumer's landed form verbatim
(unbound-force/unbound-force@`1cbb5c80`, verified from the commit) —
10 lines: a 3-line comment then seven denies:

```yaml
    # Terminal catch-all denies, placed last so they win over every
    # allow on any prefix: each shell metacharacter is a chaining,
    # redirection, or substitution vector.
    "*>*": deny
    "*<*": deny
    "*|*": deny
    "*;*": deny
    "*&*": deny
    "*$(*": deny
    "*`*": deny
```

Placement and insertion points:

- **Curator** (agents.go:47): insert after
  `"dewey gate auto-answer": allow`, before the closing `---`.
- **Distiller** (agents.go:148): insert after `"gh issue comment":
  allow`, before the closing `---`.
- The resulting map order is: `"*": deny` first, the agent's allows,
  then the terminal denies as the final entries — exactly DI-FR-003's
  required sequence.

Rationale:

- **Last placement is load-bearing.** Under last-match-wins, an allow
  placed after a terminal deny would shadow it, and a terminal deny
  not last could be shadowed by a later allow. Terminal-last is the
  only ordering where the denies override injection payloads on every
  allowed prefix.
- **Canonical order is pinned** — `*>*`, `*<*`, `*|*`, `*;*`, `*&*`,
  `*$(*`, `` *`* `` — matching the consumer lint's last-7 contract,
  DI-FR-003, and the extended shape assertions, so all three
  enforcement points agree on one sequence.
- **The consumer's text verbatim** makes dewey's scaffolded output
  converge byte-for-byte with the copies the consumer already fixed,
  so the exclusion in unbound-force/unbound-force can be dropped.
- **The comment lines are inert to every consumer**: OpenCode's YAML
  parser and the tests' `yaml.Node` walks ignore comments;
  gate.go's `verifyCuratorDefinition` extracts only the top-level
  `mode` key by line-based `Cut`/name comparison, which nested
  comment lines cannot collide with (verified against the comment
  text — no line's pre-colon segment trims to `mode`).
- **Rejected: a shared Go constant** for the block spliced into both
  templates. Two occurrences do not meet the 3+ duplication
  threshold, the map values stay readable as whole documents, and
  `slash_commands.go` precedent splices inline.

### D2 The backtick splice for the `` *`* `` key

Go raw string literals cannot contain a backtick, and the `` "*`* ":
deny `` line needs one in the emitted YAML. The templates splice it
with the technique `slash_commands.go` established — close the raw
literal, concatenate an interpreted string holding one backtick,
resume the raw literal. The tail of each template after the edit:

```go
    "*&*": deny
    "*$(*": deny
    "*` + "`" + `*": deny
---
```

Each template's single raw string literal becomes a three-part
concatenation around the backtick; the emitted bytes are exactly the
D1 block (the concatenation is source-level only).

The `deweyAgentDefinitions` doc comment (agents.go:11-12) currently
reads:

```go
// The contents are raw string literals, so an agent definition MUST
// NOT contain a backtick; use quotes for inline identifiers instead.
```

It is updated to document the single exception (exact replacement
text):

```go
// The contents are raw string literals, so an agent definition MUST
// NOT contain a backtick; use quotes for inline identifiers instead.
// The single exception is the "*`*" terminal-deny key that every
// permission.bash map ends with (scaffold-bash-terminal-denies,
// DI-FR-003): a backtick cannot appear in a raw string literal, so
// the templates splice it in through Go string concatenation — the
// `+ "`" +` technique slash_commands.go established.
```

This keeps the no-backtick rule truthful for everything a future
editor might write (prose, identifiers) while naming the one payload
that requires the splice.

### D3 Tracked mirrors and recomputed marker digests

The marker digest is the SHA-256 of the canonical body
(`canonicalBody` → `scaffoldMarker`, scaffold.go), so both mirrors'
digests change with the payload — `81a87a8e…` and `9e1c3054…` are
replaced by the digests of the new bodies. Regeneration follows the
established procedure (tool-owned-scaffold-updates, task 3.3):

1. Build the binary (`go build`).
2. Run `dewey init --vault .` at the repository root. Both mirrors are
   verifiably untouched (their markers' digests match their current
   bodies), so the `scaffoldUpdate` path rewrites them to the new
   canonical bodies with fresh markers, logging `updated agent` for
   exactly the two paths.
3. Verify with `git status` / `git diff`: only
   `.opencode/agents/curator.md` and `.opencode/agents/distiller.md`
   change — the 10-line insertion plus the marker line. The slash
   commands and workflow example are `scaffoldCurrent` (silent,
   untouched); `.uf/dewey/` config files, `.opencode/dcp.jsonc`, and
   `.gitignore` are user-owned and unchanged.

The version field in the fresh markers carries whatever the built
binary reports (`dev` for a plain `go build`), matching the existing
mirrors; the version is informational only and never affects
classification. The byte-parity tests then hold without modification
because they construct expected payloads from the embedded map at
test time (D6 of tool-owned-scaffold-updates).

### D4 Curator shape guard: the ordered assertion extends to nine entries

`assertCuratorFrontmatterShape`'s `wantBash` (cli_test.go:5891-5897)
extends from two entries to the nine-entry ordered sequence:
`{"*", "deny"}`, `{"dewey gate auto-answer", "allow"}`, then the seven
terminal denies in the canonical order, each `deny`. YAML comment
lines do not appear in `yaml.Node` content, so `bashPairs` counts
exactly the nine mapping entries and the existing entry-by-entry
comparison asserts key and value at every position. The length
mismatch error message ("the `\"*\"` deny and the single gate allow")
is updated to name the new shape (deny-all, the gate allow, and the
seven terminal denies). Both shipped copies (tracked file and embedded
map entry) are asserted through the existing callers, unchanged.

To execute the new rejection arms rather than only inspect them — the
repo's established pattern from curator-tool-allowlist —
`TestCuratorAgent_GuardsRejectDoctoredDocuments` gains two cases:

- **terminal deny deleted** → length mismatch
  (`permission.bash has 8 entries, want exactly 9`);
- **an allow moved after the terminal denies** → the ordered
  entry-comparison mismatch.

A guard arm that never runs against a failing document is untested;
these cases keep the new arms executed by construction.

### D5 Distiller shape guard: last-7 semantics mirroring the consumer lint

`assertDistillerFrontmatterShape`'s bash block
(cli_test.go:6318-6336) is extended from set-based checks to the
consumer lint's contract:

- **Last-7 assertion (ordered)**: the mapping's final seven `bashPairs`
  entries are exactly the seven terminal deny keys in the canonical
  order, each with value `deny`.
- **Allow preservation**: the existing allow loop gains
  `gh issue comment` — DI-FR-004's scenario names it, and the current
  loop covers only the two startup gates.
- The existing `"*": deny` check is kept.

Because no distiller doctored-documents test exists, one compact
doctored case is added (a subtest using the shared `problemRecorder`
infrastructure, `testing.TB`-generic): drop one terminal deny from a
copy of the distiller document and assert the last-7 arm reports the
mismatch — mirroring D4's rationale for the distiller guard.

### D6 Allowed commands remain runnable (DI-FR-004 analysis)

The three allow entries — `dewey gate auto-answer`, `dewey gate
distill-unattended`, `gh issue comment` — contain none of `>`, `<`,
`|`, `;`, `&`, `$(`, or a backtick. Under last-match-wins the terminal
denies therefore never match the bare allowed commands: for those
command strings the allow entry remains the last matching entry and
each resolves to `allow`. An injection payload on an allowed prefix
contains a metacharacter, so the matching terminal deny — placed after
every allow — is the last matching entry and wins. No allow entry is
added, removed, or reordered; the startup gates and the dispatch-issue
comment path keep working (the spec's "Startup gates remain runnable"
and "Injection payloads on allowed prefixes are denied" scenarios).

### Unchanged surfaces (explicit)

- Marker scheme, classification table, refresh / skip-customized
  semantics, symlink and error handling in `scaffold.go` — untouched;
  the change only supplies new map values to the existing machinery.
- User-owned scaffolds (`.uf/dewey/` config, `dcp.jsonc`, `.gitignore`)
  — untouched.
- gate.go's `verifyCuratorDefinition` — unaffected (D1 comment-line
  analysis).
- Prompt-text allowlist parity tests
  (`TestCuratorAgent_AllowlistParity`,
  `TestDistillerAgent_AllowlistParity`) and
  `TestDistillerAgent_QuestionDenied` — parse the body text or do
  string-contains checks, not the bash map; unaffected.
- Byte-parity and first-init tests — construct expected payloads from
  the map; adapt automatically once mirrors are regenerated.
- No new dependency; pure content plus test change.

## Risks / Trade-offs

1. **Marker digest churn is by design.** Both mirrors' digests change
   with the payload. Untouched drifted consumer copies refresh on
   their next `dewey init` (`scaffoldUpdate`); customized copies are
   preserved. The digest is not a public contract — no consumer pins
   it.
2. **The `` *`* `` deny blocks legitimate backtick use** on allowed
   prefixes (shell quoting constructs). Accepted: the agents' allowed
   commands never need backticks, and the deny is the security
   boundary the consumer lint already mandates.
3. **The 10-line block is duplicated across four copies** (2 embedded
   + 2 tracked), inherent to the byte-parity two-copy discipline; the
   parity tests hold it, and the rejected-constant alternative (D1)
   would trade duplication for fragmented templates.
4. **Mid-sequence red tests.** The embedded templates, tracked
   mirrors, and shape guards are three-way coupled: between editing
   `agents.go` and regenerating the mirrors, the parity tests fail;
   between regenerating and extending the guards, the curator shape
   test fails (length 9 ≠ 2). The task phases acknowledge this — the
   full suite is the checkpoint after the guards land, not after the
   template edit alone.
5. **Doctored-case additions grow the test tables** — trivial,
   standard-library only, and they buy executed rejection paths for
   the new arms.

## Constitution Alignment

The proposal records all four principles as PASS; the design keeps
them:

- **Autonomous Collaboration (I):** the change is confined to the
  content of artifacts dewey already emits, guarded by the existing
  scaffold marker scheme — no runtime coupling is introduced, and the
  permission map each agent carries continues to be the enforcement
  boundary, now accurately describing the agent's real shell surface
  (D1, D6).
- **Composability First (II):** no new dependency; the added entries
  are inert YAML to any consumer that ignores them, and consumers not
  running OpenCode's permission engine are unaffected (Unchanged
  surfaces).
- **Observable Quality (III):** the emitted payload stays verifiable
  through the recomputed `dewey:scaffold` marker digests (D3), and the
  security posture becomes machine-checkable — the extended shape
  guards assert the exact terminal-deny suffix, matching the consumer
  lint's last-7 contract (D4, D5). No provenance metadata is weakened;
  init's outcome events are unchanged.
- **Testability (IV):** pure content verifiable in isolation with the
  standard library `testing` package — `yaml.Node` order-preserving
  parses pin the sequence, parity tests compare bytes against payloads
  constructed from the embedded map, and the doctored fixtures execute
  the guards' rejection paths without any external service (D4, D5).

## Assumptions

### A1 The consumer's landed block is the normative text

The 10-line block was extracted verbatim from
unbound-force/unbound-force@`1cbb5c80` (both agent files in that
commit carry byte-identical blocks). Any future edit to the comment
wording or deny order must update all four copies together and
re-derive the markers — the parity tests enforce this.

### A2 The consumer lint's last-7 contract is as the proposal describes

The instruction-lint test name and required entries are taken from the
proposal (verified against the consumer commit's message and diff);
the lint itself lives in the consumer repository and is not executed
by dewey's CI. Dewey's own guards (D4, D5) encode the same invariant
so convergence is checkable here.

### A3 `yaml.Node` ordering is authoritative for the guards

Both shape guards already rely on `yamlMappingPairs` walking
`yaml.Node.Content` in document order and on comments being absent
from that walk; the new assertions depend on the same property. The
duplicate-key `t.Fatalf` in `yamlMappingPairs` also protects the new
sequence against accidental key duplication.
