<!-- originating_issue: 147 -->

## Why

`lint` treats a decision learning as stale after 30 days
(`staleDecisionThreshold`). Age is a weak proxy: a week-old learning
can be wrong after one merge, and a year-old one can still be exact.
With #146, learnings carry an `as_of` pin and `evidence` locators, so
staleness can be detected from the code itself.

In the knowledge-base effort that motivated #153, the equivalent
"existence pre-pass" resolved every cited path at the pins and queued
exactly the claims whose cited files had changed for re-verification.

Depends on #146. Part of #153 (umbrella).

## What Changes

Two new deterministic `lint` checks over learnings that carry `as_of`
and `<repo>:<path>` evidence for the vault's own repository:

- **`anchor_missing`** — the cited path does not exist at the `as_of`
  commit, or the `as_of` commit is not in the repository.
- **`anchor_drift`** — the cited path changed between `as_of` and
  `HEAD` (including deletion).

`lint` with `fix: true` sets `validity: stale-unverified` on learnings
with drift, in the store and in their markdown backup. It never
changes tiers or deletes content.

The 30-day `stale_decision` check is unchanged and remains as a
secondary signal; anchor checks are the primary staleness signal for
pinned learnings.

## Capabilities

### New Capabilities
- `anchor checks`: `anchor_missing` and `anchor_drift` lint kinds.

### Modified Capabilities
- `lint`: two new finding types and summary counters; `fix` also
  marks drifted learnings `stale-unverified`.

### Removed Capabilities
- None.

## Impact

- `tools/lint_anchors.go` (new) — locator parsing, git-backed checks
  behind an interface, fix.
- `tools/lint.go` — wiring, summary counters, `anchor_checks`
  outcome, fix result and warnings.
- `tools/learning_temporal.go` — the markdown-backup write and the
  properties decoder are shared with the #146 supersession flow
  (`updateLearningBackup`, `decodeLearningProperties`).
- Tests: `tools/lint_anchors_test.go` (real temp git repository plus
  a fake checker for error paths).
- README lint section and MCP tools table; AGENTS.md CLI table and
  Recent Changes.
- Website tracking: docs sync website#398, blog website#399.

**Not checked:** `<repo>#<n>` locators (resolving them needs the
GitHub API; Dewey processes locally by default), locators for other
repositories (#106), learnings pinned to another repository,
learnings without `as_of`, and learnings whose validity is
`superseded` or `historical`.

## Constitution Alignment

Assessed against the Dewey project constitution (v1.4.0).

### I. Composability First

**Assessment**: PASS

Local `git` only; no network. Skipped entirely without a vault path
or outside a git repository.

### II. Autonomous Collaboration

**Assessment**: PASS

Findings name the learning, the anchor, and the commit range, so an
agent can re-verify and store a superseding learning.

### III. Observable Quality

**Assessment**: PASS

Machine-parseable findings and summary counters; fixes are reported.
Every result carries `anchor_checks` (`checked`, `skipped`, or
`failed`, with a reason), so a skipped or failed pass is never read
as clean, and backup failures during `fix` are reported in
`anchor_fix_warnings`.

### IV. Testability

**Assessment**: PASS

Git access sits behind an interface; tests use a real temporary
repository and a fake for failure paths.
