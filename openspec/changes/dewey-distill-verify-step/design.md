## Context

`/dewey-distill` (#145) runs lint, inventory, a mandatory scope
gate, compile/curate through the running MCP server, a mandatory
review gate, batch `promote`, and a report. The human at the review
gate sees only what compile and curate produced; nothing says
whether a candidate's cited code still exists or changed since its
pin.

Anchor-drift lint (#147) already answers that question
deterministically. `tools/lint_anchors.go` parses `<repo>:<path>`
evidence locators, resolves each learning's `as_of` pin, and uses a
local-git-backed `anchorChecker` to report `anchor_missing` and
`anchor_drift`. The mechanism is not surfaced per candidate at the
review gate.

The promotable queue is tier `draft` pages in `learning/`,
`compiled/`, and `knowledge/`. `promote` resolves names against
all three namespaces and gates on tier `draft`, not on namespace,
so the verify queue enumerates the same set — a knowledge store
configured with `trust_tier: draft`, or any draft page re-ingested
under `knowledge/`, is as promotable as a draft learning. Learnings
carry `as_of` and `evidence`; compiled articles carry `sources`
(learning identities), `tier`, and `compiled_by`, but no pin of
their own. The curator agent (`agent: curator`, #152) runs the
command with a fixed Dewey MCP tool allowlist stated by CA-FR-003.

## Goals / Non-Goals

### Goals

- Add a deterministic, non-mutating verification step between
  compile/curate and the review gate.
- Present each candidate with identity, tier, evidence locators,
  and a result from a fixed enum.
- Reuse the existing anchor checker rather than reimplementing git
  access.
- Keep promotion a human decision; make the gate cheap, not
  automatic.

### Non-Goals

- Auto-promotion, tier changes, or validity changes from
  verification.
- Resolving `<repo>#<n>` locators (network).
- Multi-repository anchors (#106).
- Semantic re-verification of the claim itself, or an endorsement
  judgment.
- Changing lint's existing anchor behavior or the 30-day check.
- Hard-scoping the new tool through V2 agent `permissions` rules.

## Decisions

### D1 Tool shape and scope selection

A new `verify_candidates` MCP tool (KV-FR-001) takes optional
`pages []string` and `tags []string`. Supplying both is a
validation error, mirroring `promote`'s `page`/`pages` rule.
`pages` names explicit candidates; `tags` filters by tag; with
neither, the tool verifies every promotable candidate. Tag
membership is read from page-name prefixes (`learning/{tag}-*`,
`compiled/{tag}`) and the `tag` property, exactly as the command's
inventory step does; prose is never a tag source. Matching is
exact in both forms: the `tag` property must equal the requested
tag, and the page-name form requires the delimiter after the tag
(`learning/auth-…`, never `learning/authentication-…` matched by
`auth`); `knowledge/` page names carry no tag prefix, so they
match only through the `tag` property. Duplicate pages are
deduplicated.

Only tier `draft` pages are promotable, so only draft pages are
enumerated. A requested page that is missing or not draft appears
as a `skipped` entry with a reason; it is never silently dropped,
because the reviewer needs to know a candidate vanished.

### D2 Reuse the anchor checker

`Verify` holds the same `anchorChecker` interface and pass-budget
shape as `Lint` (`tools/lint_anchors.go`), constructed as a
`gitAnchorChecker{dir: vaultPath}`. Learning eligibility reuses
`learningAnchors`: an `as_of` pin to the vault's own repository, at
least one `<repo>:<path>` anchor, and a validity other than
`superseded` or `historical`. Evidence limits are re-checked at
consumption, because re-ingested frontmatter bypasses
`store_learning`'s validation. No git command is duplicated; the
existing runner's timeouts, literal pathspec, and
`--no-ext-diff --no-textconv` protections apply unchanged.

**DRY decision — pass runner not extracted.** The per-anchor loop
that enforces the budget and timeouts ("the pass runner") exists
twice after this change: once in `Lint.checkAnchors` and once in
`Verify`. Extracting a shared runner is deliberately deferred: the
two loops emit different shapes (lint findings versus candidate
outcomes) over different page sets, a generic runner would need a
callback seam wide enough to erase the clarity it buys, and two
consumers do not yet meet the repo's rule-of-three extraction bar.
The genuinely shared surface — `anchorChecker`, `learningAnchors`,
`parseAnchor`, and the git invocation protections — is reused, not
duplicated. A third consumer (for example a `doctor` anchor pass)
is the trigger to revisit extraction.

### D3 Compiled provenance aggregation

A compiled article has no pin, so it cannot be checked directly.
Its `sources` name the learnings it synthesized. The tool resolves
those source learnings and aggregates: `missing` if any source is
`missing`, else `drifted` if any source is `drifted`, else
`anchors_ok` if any source is `anchors_ok`, else `no_anchors`.
Presented evidence is the deduplicated union of the sources'
locators. The entry also reports the `sources` identities and their
individual results as additive detail, so the aggregate is
auditable rather than opaque.

An unresolvable source contributes nothing; if no source resolves
to a stored, checkable learning, the article is `no_anchors`.
Reporting the per-source identities and results is a MUST, not
decorative detail (KV-FR-003): without them, a `drifted` aggregate
forces the reviewer to open every candidate source by hand to
find which one drifted, which is exactly the manual re-checking
this change exists to remove from the gate.

### D4 Result enum and pass status

Every candidate carries exactly one of `anchors_ok`, `drifted`,
`missing`, `no_anchors`, or `skipped` with a reason (KV-FR-002).
The pass carries `checked`, `skipped`, or `failed`. A candidate
with evidence but no locator checkable in this repository is
`no_anchors` and the pass stays `checked`; absence of anchors is a
fact about the candidate, not a failure.

`skipped` means the pass could not run (for example, the vault is
not a git repository). `failed` means git could not answer, the
request was cancelled, or the budget was exhausted; it is returned
as an error result (`IsError`) so a caller cannot read an unrun
pass as clean. The error routing is the load-bearing detail:
whatever candidates were checked before the failure ride in the
error result only as context, and MUST NOT be presented as
verified — a half-run pass is a failure, not a shorter success.
This mirrors lint's `anchor_checks` outcome
(`checked`/`skipped`/`failed` with a reason).

### D5 Non-mutation

The tool is read-only (KV-FR-005). It reads pages and runs git
queries; it never writes the store, a markdown backup, or a page
property, and never calls `promote`. It does not pre-select
candidates. This is the load-bearing guarantee behind "verification
is not endorsement": a passing check cannot advance a tier.

### D6 Determinism and boundedness

Candidates are ordered deterministically (namespace, then page
name). Runs over identical store content, pin, and `HEAD` that
each complete a `checked` pass return identical results; a run
that ends `skipped` or `failed` — budget exhaustion, request
cancellation, git failure — is a failure mode and is excluded from
the determinism claim, because whether the budget runs out depends
on wall-clock timing, not on store content. Git access is local
only, with no network call. Each git call keeps the anchor timeout
and one pass is bounded by the pass budget; both are injectable
fields so tests can expire them without waiting, as in lint. The
request context is honored between calls.

### D7 Command integration and the human gate

The command gains a verify step (KD-FR-104) between compile/curate
and the review gate. It makes one `verify_candidates` call with the
run's scope and presents the result; it does not hand-check anchors
and does not shell out. The scope is the run's candidate queue —
the tags the human approved at the scope gate, or the pages the run
produced across `learning/`, `compiled/`, and `knowledge/`. The
review gate (KD-FR-105) shows each candidate's identity, tier,
evidence, and result before the existing `HUMAN_GATE:` question,
and nothing is promoted without the human's selection. Every
`skipped` entry — candidate or pass — is presented with its reason.
The presentation states code correspondence only: `anchors_ok`
means the cited paths exist and are unchanged, never that the claim
is endorsed or worth promoting. When the pass is `skipped` or
`failed` (KD-FR-106), the command states that plainly and does not
present candidates as verified clean; the gate still runs.
Verification never promotes (KD-FR-107).

The command file stays byte-identical to the `deweySlashCommands`
entry, so the scaffold parity rule from CA-FR-005 is preserved.

### D8 Curator allowlist extension

The command runs under `agent: curator`, so the allowlist gains
exactly `verify_candidates` (CA-FR-013) and nothing else.
`verify_candidates` is read-only, so the curator's mutation
boundary is unchanged (CA-FR-014). The sibling
curator-tool-allowlist change made the allowlist frontmatter-
enforced (CA-FR-009), so this extension adds
`dewey_verify_candidates: true` to the `tools:` map in both shipped
copies, after the `"dewey_*": false` deny, and keeps the
prompt-text allowlist and the frontmatter allow set in parity
(CA-FR-011). The command's tool discipline, tracked-file
assertions, and the manual acceptance matrix remain; the V2
`permissions` conversion stays the future hard-enforcement path.

Two provenance notes. First, the extension carries a recorded
human authorization (see proposal.md, "Recorded Human
Authorizations"): agent tool-access settings are a protected
gatekeeping category, and the authorization — issued at the
2026-11-11 spec-review pause for issue #151 — is recorded in the
change artifacts so the edit is auditable rather than silent.
Second, the allowlist text lives in two tracked places:
`.opencode/agents/curator.md` and the `agents.go` scaffold mirror
that `dewey init` writes. Both are edited in the same commit and
must stay byte-identical, exactly as the command file and its
`slash_commands.go` mirror do (CA-FR-005's parity rule, applied to
the agent file); the cli_test parity assertions cover both
mirrors.

### D9 Seam verification

Per DE-007, the implementation increment verifies three seams in
the same change: registration surface (the tool is registered with
an input schema matching `VerifyCandidatesInput`), documented
output shape (a golden-shape test over a `checked` result), and
round-trips (verification leaves store content and tiers
unchanged). Tasks.md names each seam layer explicitly.

## Risks / Trade-offs

- **One git subprocess per anchor.** Inherited from lint. Acceptable
  at current vault sizes and bounded by the pass budget; a batched
  `git diff --name-only` per distinct pin can come later.
- **Aggregation can hide which source drifted.** Mitigated by
  reporting per-source identities and results alongside the
  aggregate; the reviewer can open the source learning.
- **Rename shows as drift.** A moved file reports drift at its old
  path, which correctly prompts re-verification.
- **Scope can drift between verify and promote.** A page promoted
  or deleted between the two steps is reported `skipped` by verify;
  `promote` remains all-or-nothing and fails closed on it.
- **The gate is slower.** The verify call adds local git work; the
  bounded budget keeps it finite, and a skipped or failed pass
  degrades to the existing gate rather than blocking it.
- **V1 allowlist is policy-enforced, not host-enforced.** The same
  residual risk CA-FR-003 already records; the read-only tool adds
  no mutation surface.
