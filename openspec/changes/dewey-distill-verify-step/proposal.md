<!-- originating_issue: 151 -->

## Why

`/dewey-distill` (#145) runs lint, inventory, a scope gate,
compile/curate through MCP, a review gate, batch `promote`, and a
report. The human at the review gate currently sees only what
compile and curate produced: article names, topics, source counts,
and store item counts. Nothing tells the reviewer whether a
candidate's cited code actually still exists or has changed since
the learning was pinned. The reviewer must either trust the
candidate blindly or re-check it by hand, which makes the gate
expensive right when batch `promote` is all-or-nothing.

Anchor-drift lint (#147) already answers the code-correspondence
question deterministically: it reuses local git to compare each
`<repo>:<path>` evidence locator at the learning's `as_of` pin and
at `HEAD`. That mechanism is not surfaced per candidate at the
review gate, so the human cannot see, page by page, whether the
claim still matches the code.

This change adds a deterministic verify step between compile/curate
and the review gate. It runs anchor checks over every candidate in
scope and presents each with its identity, tier, evidence
locators, and a verification result. Promotion stays human. The
goal is to make the human gate cheap, not to remove it.

Verification is not endorsement. A passing anchor check says a
cited path still exists and is unchanged; it does not say the claim
is correct, worth acting on, or safe to trust. Endorsement remains
a human judgment.

### Why not auto-promote

Learning files are untrusted backups. A file claiming
`tier: validated` is re-ingested as `draft` until a human promotes
it (#145). Auto-promotion after a correspondence check would open a
path for injected source content to reach `validated` — the tier
review agents trust. A deterministic check can confirm that a path
exists and did not change; only a human can decide that the claim
is worth endorsing. This change therefore keeps every tier
transition behind explicit human selection.

Part of #153. Depends on #147 (anchor-drift lint, landed) and #145
(`/dewey-distill` and batch promote, landed).

## What Changes

- **New deterministic verification tool** (`verify_candidates`):
  a non-mutating MCP tool that enumerates the promotable
  candidate queue — every tier `draft` page under `learning/`,
  `compiled/`, and `knowledge/` in scope, matching `promote`'s
  tier-based eligibility across all three namespaces it resolves
  against — reuses the existing `anchorChecker` from
  `tools/lint_anchors.go`, and returns one entry per candidate
  with its page name, identity, tier, evidence locators, and a
  verification result. It never changes a tier, a validity, a
  page, or a file, and never promotes anything.
- **Verification result enum** on the tool result:
  `anchors_ok`, `drifted`, `missing`, `no_anchors`, and `skipped`
  (with a reason). A candidate with no checkable locator is
  `no_anchors`, not a failure; a pass that cannot run is `skipped`
  or `failed` and is never presented as clean. A `failed` pass
  routes as an error result (`IsError`) so no caller can read an
  unrun pass — or the candidates checked before the failure — as
  verified clean. Determinism is scoped to runs that complete a
  `checked` pass: budget exhaustion, cancellation, and git
  failure are failure modes, not differing deterministic results.
- **Scope selection**: the tool accepts either explicit `pages`
  or `tags`, or neither (all promotable candidates). It mirrors
  the command's existing tag reading — page-name prefixes plus
  the `tag` property — and never infers tags from prose. Tag
  matching is exact: a page matches a requested tag only when
  the page's `tag` property equals the tag or the page name
  carries the tag with its delimiter (`learning/{tag}-…`,
  `compiled/{tag}`); `knowledge/` page names carry no tag prefix,
  so they match only through the `tag` property. A tag never
  matches by substring or by prefix without its delimiter
  (`auth` does not match `authentication`).
- **Compiled-article provenance**: a compiled article carries
  `sources` (learning identities), not its own `as_of`/`evidence`.
  Its verification result aggregates the anchor results of its
  source learnings, and its presented evidence is the union of
  their locators. Each source identity and its individual result
  are reported alongside the aggregate so the aggregation is
  auditable, not opaque.
- **`/dewey-distill` verify step**: a new step between
  compile/curate and the review gate calls `verify_candidates`
  once with the run's scope and presents the result per candidate
  before asking the human what to promote. The review gate still
  runs on a non-pass verification, but states plainly that
  verification was skipped or failed; it is never shown as clean.
  Every `skipped` entry — candidate or pass — is presented with
  its reason. A presented result states code correspondence only;
  it MUST NOT be phrased as an endorsement or a recommendation
  to promote.
- **Review gate stays human**: the verify step is informational and
  pre-gate only. It MUST NOT promote, pre-select, or alter the
  existing `HUMAN_GATE:` review question. Nothing is promoted
  without explicit human selection.
- **Curator allowlist extension**: the curator agent's Dewey MCP
  tool allowlist gains `verify_candidates`, because the command
  runs under `agent: curator`. No other allowlist entry and no
  permission rule changes. The tracked `.opencode/agents/curator.md`
  and its `agents.go` scaffold mirror are edited together so
  `dewey init` scaffolds the extended allowlist byte-identically.
  This extension carries a recorded human authorization — see
  "Recorded Human Authorizations" below.
- **No auto-promotion and no tier claims**: the change adds no
  path by which a source document, a learning file, or a compiled
  article can reach `validated` without a human `promote` call.

## Capabilities

### New Capabilities

- `knowledge-verification`: the `verify_candidates` MCP tool —
  candidate enumeration and scope, the per-candidate output
  contract and result enum, reuse of the anchor checker, compiled
  provenance aggregation, degraded-mode behavior, and the
  non-mutation and determinism guarantees.

### Modified Capabilities

- `knowledge-distillation`: the `/dewey-distill` workflow gains a
  deterministic verify step between compile/curate and the review
  gate, and the review gate presents per-candidate verification
  results before the human selects pages. The capability has no
  archived main spec yet, so its delta adds requirements alongside
  the pending "Gated Distill Workflow" contract from the
  `dewey-distill` change rather than modifying it.
- `curator-agent`: the Dewey MCP tool allowlist is extended to
  include `verify_candidates`. The capability has no archived main
  spec yet, so its delta adds the extension alongside the
  `curator-agent-human-gates` change's CA-FR-003 allowlist.

**Archive-time reconciliation**: both sibling changes this delta
sits alongside — `dewey-distill` (the "Gated Distill Workflow"
contract) and `curator-agent-human-gates` (CA-FR-003's allowlist) —
are pending, not archived. When this change archives, the archive
step MUST reconcile against whatever has landed first: if
`curator-agent-human-gates` has already archived, CA-FR-009's
additive allowlist extension is folded into the archived
CA-FR-003 text (or kept as the additive CA-FR-009 requirement with
a cross-reference), never as a silent contradiction; if
`dewey-distill` has already archived, KD-FR-104's insertion point
(the step order inside "Gated Distill Workflow") is reconciled so
the archived pipeline description reads coherently with the verify
step between compile/curate and the review gate. The requirement
ids stay inside the ranges each delta header allocates, so
numbering collisions cannot occur; only prose reconciliation is
in scope.

### Removed Capabilities

- None.

## Impact

- `tools/verify.go` (new) — the `verify_candidates` handler,
  candidate enumeration, anchor reuse, provenance aggregation, and
  result rendering.
- `tools/verify_test.go` (new) — real temporary git repositories
  plus a fake `anchorChecker` for degraded and failure paths.
- `types/tools.go` — a new `VerifyCandidatesInput` with `pages`
  and `tags`.
- `server.go` — register the tool and construct the handler with
  the vault path.
- `.opencode/commands/dewey-distill.md` — the new verify step, the
  renumbered subsequent steps, and the review gate presentation.
  The tracked command file and the `deweySlashCommands` entry
  (`slash_commands.go`, the `dewey-distill.md` scaffold) stay
  byte-identical.
- `.opencode/agents/curator.md` — the allowlist gains exactly one
  tool; every other tool flag and permission rule is unchanged.
- `agents.go` — the embedded `curator.md` scaffold mirror is edited
  in the same commit so `dewey init` scaffolds the extended
  allowlist byte-identically to the tracked agent file.
- `README.md` and `AGENTS.md` — the MCP tool table and the
  distill/lint documentation. The exact edits are enumerated in
  tasks.md 4.1 and 4.2: README.md lines 29, 765, 876, and 904, and
  AGENTS.md lines 5, 338, 343, and 371 (tool counts 50 → 51, the
  verify step in the `/dewey-distill` pipeline descriptions, the
  `tools/` package comment, and the server registration count).
- Tests: enumeration and scope (including `knowledge/` pages and
  exact tag matching), result enum, provenance aggregation with
  per-source identities, non-mutation, non-repository vault, git
  failure and budget exhaustion, request cancellation, failed-pass
  error routing, and registration surface parity.
- No database schema change, no new module dependency, no CGO,
  and no change to the 37 inherited graphthulhu tools.
- Website tracking: extend the open `unbound-force/website#371`
  sync issue (dewey-distill documentation) with the new tool and
  the verify step, rather than opening a new issue — the docs
  pages it tracks are the same pages this change touches.

## Recorded Human Authorizations

Two decisions below deviate from a standing repo rule or touch a
protected gatekeeping category (AGENTS.md, "Gatekeeping Value
Protection"). Both were made by the human (jflowers) during the
2026-11-11 spec-review pause on this change, relayed with the
finding set, and are recorded here with provenance so the
deviation is auditable rather than silent. They are narrow,
final, and not precedent for future changes.

1. **Workflow tier: OpenSpec retained despite the "new MCP tools →
   Speckit Always" row** (AGENTS.md, Strategic vs Tactical table).
   `verify_candidates` is a new MCP tool, which normally routes
   this change to Speckit. The human authorized keeping it on the
   OpenSpec tier, given the bounded, read-only scope of the tool
   (one non-mutating query tool over an existing seam) and because
   the review council already exercised the strategic checks a
   Speckit pipeline would add. Provenance: human decision at the
   spec-review pause for issue #151, 2026-11-11 — "Apply all
   recommended fixes, keep OpenSpec."
2. **Curator allowlist extension** (AGENTS.md Gatekeeping
   category 5 — agent tool-access settings). CA-FR-009 adds the
   read-only `verify_candidates` tool to the curator agent's Dewey
   MCP tool allowlist. The human explicitly authorized this
   extension: the tool adds no mutation surface, so the extension
   is not gate-weakening, and recording it here keeps the edit
   inside the documented authorization path. Provenance: human
   decision at the spec-review pause for issue #151, 2026-11-11.

## Constitution Alignment

Assessed against the Dewey constitution (v1.4.0).

### I. Composability First

**Assessment**: PASS

The tool uses local `git` only, with no network. It is additive:
`/dewey-distill` degrades to a plain review gate when verification
is skipped or unavailable, and nothing outside Dewey depends on it.
No new module dependency and no CGO.

### II. Autonomous Collaboration

**Assessment**: PASS

The verify step is an MCP tool call with a documented input schema
and a structured JSON response, not a shell command or an internal
call. The human still decides promotion through the existing
question tool and `promote`. The command layer holds no judgment-
free orchestration that belongs in tool code.

### III. Observable Quality

**Assessment**: PASS

Every candidate carries its identity, tier, evidence locators, and
a result from a fixed enum. The result reports whether the pass was
`checked`, `skipped`, or `failed` with a reason, so "verified clean"
is distinguishable from "never checked". The verification result
adds provenance without claiming endorsement.

### IV. Testability

**Assessment**: PASS

Git access sits behind the existing `anchorChecker` interface, so
tests use a real temporary repository for the happy path and a fake
for degraded and failure paths. Candidate enumeration, scope
filtering, provenance aggregation, and non-mutation are testable
against an in-memory SQLite store with no external service.
