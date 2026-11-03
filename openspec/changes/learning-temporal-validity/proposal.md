<!-- originating_issue: 146 -->

## Why

Learnings record `created_at` but nothing about the code state they
were true against. A learning about `tools/lint.go` is equally
"fresh" one minute and one refactor later. Consumers cannot tell a
claim verified at today's HEAD from one that predates three merges,
and Dewey cannot later detect drift (#147) or weigh validity at
retrieval time (#148) without a pin to compare against.

Evidence from a 347-claim knowledge base compiled from two Dewey
stores: pinning each claim to `repo@sha` with evidence locators made
claim-by-claim verification against code possible, and separated
claims true only on open-PR branches from claims true on main —
a distinction `created_at` cannot express.

Part of #153 (umbrella).

## What Changes

`store_learning` gains temporal-validity metadata:

- **`as_of`** (automatic): `<repo>@<HEAD sha>` for the vault's git
  repository, resolved with a bounded `git` subprocess. Omitted when
  the vault is not a git repository or git is unavailable.
- **`evidence`** (optional input): a bounded list of locator strings
  (`<repo>:<path>`, `<repo>#<n>`, `learning/<identity>`,
  `session:<id>`).
- **`validity`** (optional input): `current`, `stale-unverified`,
  `superseded`, or `historical`. Absent means unknown.
- **`supersedes`** (optional input): the identity of an existing
  learning this one replaces. The target is linked back: it gains
  `superseded_by` and `validity: superseded`, in the store and in its
  markdown backup. Superseded learnings are never deleted.

All fields are written to the learning's properties and its markdown
frontmatter, and round-trip through startup re-ingestion. Read tools
already return page properties, so the fields are visible without
read-path changes.

Validity is an attribute, not a tier: tiers keep meaning "who vouched
for this".

## Capabilities

### New Capabilities
- `learning temporal validity`: `as_of` pinning, `evidence`,
  `validity`, and two-way `supersedes` / `superseded_by` links on
  learnings.

### Modified Capabilities
- `store_learning`: accepts `evidence`, `validity`, `supersedes`;
  stamps `as_of`; returns the new fields.
- `reIngestLearnings`: parses and restores the new frontmatter fields.

### Removed Capabilities
- None.

## Impact

- `types/tools.go` — `StoreLearningInput` fields.
- `tools/learning.go` — validation, `as_of` resolution (injectable),
  frontmatter, supersession link.
- `tools/learning_temporal.go` (new) — pure helpers for the above.
- `main.go` — `learningFrontmatter` fields and re-ingestion.
- `server.go` — tool description.
- Tests: `tools/learning_test.go`, `tools/learning_temporal_test.go`,
  `main_test.go`.
- Docs: README `store_learning` section, AGENTS.md if structure
  changes.

**Backward compatibility:** all inputs are optional; existing learning
files without the fields re-ingest unchanged. No SQL schema change —
everything lives in the existing `properties` JSON column. The tool
response gains fields; none are removed.

**Out of scope:** drift detection (#147), validity-aware retrieval
(#148), supersession in `compile` (#150), and multi-repo pinning when
a vault indexes sibling repositories (#106) — `as_of` pins the vault's
own repository.

## Constitution Alignment

Assessed against the Dewey project constitution (v1.4.0).

### I. Composability First

**Assessment**: PASS

No new dependencies. `as_of` degrades to absent without git; every
other field is optional input. Works with and without embeddings.

### II. Autonomous Collaboration

**Assessment**: PASS

Learnings become more self-describing: the code state they were true
against and the evidence behind them travel with the artifact, in the
store and in the committed markdown backup.

### III. Observable Quality

**Assessment**: PASS

Provenance (`as_of`, `evidence`) is machine-parseable and returned by
every read tool via page properties. Supersession is recorded, never
destructive.

### IV. Testability

**Assessment**: PASS

The git resolver is injected (the `resolveAuthor` pattern); helpers
are pure; tests use temp directories and in-memory stores.
