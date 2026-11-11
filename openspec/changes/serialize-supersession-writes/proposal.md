## Why

Two concurrent `store_learning` calls that supersede the same target
can both pass validation and then silently fork the supersession
chain — the one-successor invariant ("chains, not forks",
learning-temporal-validity design D3) is enforced check-then-act, not
at write time.

The guard runs during validation: `resolveSupersedesTarget` reads the
target's `superseded_by` inside `prepareTemporal`
(`tools/learning_temporal.go`). The writes land seconds later, in two
legs, after the successor's markdown file write, store insert, block
persist, and embedding generation:

- **Store leg** — `linkSuperseded` re-reads the target's properties
  under `learningPropsMu` but its mutate closure sets
  `superseded_by` unconditionally, so the second writer's `UpdatePage`
  overwrites the first successor. The mutex (added with #147 for lint
  fix versus supersession lost updates) serializes the
  read-modify-write; it does not re-check the one-successor guard,
  and it cannot see writers that bypass it.
- **File leg** — `markSupersededFile`'s read-modify-write on the
  target's markdown frontmatter replaces `superseded_by` blindly,
  with no lock at all.

The store lock serializes *processes*; the race is between concurrent
in-process MCP tool calls under streamable HTTP sharing one store
handle. Impact is metadata-only (a forked chain plus a dangling
`supersedes` claim; nothing is deleted), but it violates the
documented invariant, emits no `supersedes_warning`, and — because
`reIngestLearnings` restores only pages missing from the store — a
stale file leg silently wins after `graph.db` loss.

Deferred three times, each time routing here: PR #156 review feedback
(thread on `tools/learning_temporal.go`), a LOW concurrency finding
from the review council on the fix commit, and a MEDIUM concurrency
advisory retained by the `/uf.address-feedback` Tier 2 dispatch.
Tracked in #166.

## What Changes

- The supersession **store leg becomes a compare-and-swap**: a guarded
  conditional `UPDATE` on the target's `properties` JSON (predicate on
  `json_extract(properties, '$.superseded_by')` being absent, plus a
  row-identity guard) and a rows-affected check, following the PR-145
  toctou pattern (CAS preferred over lock widening). A writer whose
  claim does not land gets a `supersedes_warning` naming the winning
  successor; its learning is kept with its `supersedes` claim intact.
- The **file leg is guarded**: `markSupersededFile` re-reads the
  target's frontmatter and refuses to replace a `superseded_by` that
  already names a different successor; the file is left unchanged and
  the refusal reported through `supersedes_warning`. The file
  read-check-write is serialized in-process by holding
  `learningPropsMu` across each caller's store and file legs
  (supersession and lint fix).
- **Sequential semantics are unchanged**: a caller that supersedes an
  already-superseded target still fails validation with the existing
  "already superseded by" error before anything is written.
- **Race regression test** (concurrent goroutines superseding the same
  target, no sleeps, `-race`) and a **re-ingestion seam test** that
  chains the real guarded file output into the real
  `reIngestLearnings` parser after `graph.db` loss.

## Capabilities

### New Capabilities
- `serialize-supersession-writes`: atomic same-target supersession
  claims — exactly one successor survives concurrent `store_learning`
  calls across both the store leg and the markdown backup leg, with the
  loser warned and durability preserved through re-ingestion.

### Modified Capabilities
- `learning-temporal-validity`: the Two-Way Supersession requirement's
  one-successor invariant becomes write-time-enforced (guarded store
  claim plus guarded file replace) instead of validation-time-only;
  a concurrent losing writer now receives `supersedes_warning` naming
  the winner instead of silently forking the chain.

### Removed Capabilities
- None.

## Impact

- `store/store.go` — new guarded-claim method using `json_extract` in
  the `UPDATE` predicate (JSON1 already used by
  `GetPagesWithProperty`); no schema migration, no new dependency.
- `tools/learning_temporal.go` — guarded store leg (in-closure
  fast-fail plus the SQL claim), frontmatter key reader, guarded backup
  write, `learningPropsMu` extended over the file leg.
- `tools/lint_anchors.go` — `markStaleUnverified`'s store and file
  legs share one `learningPropsMu` hold; outcomes unchanged.
- `tools/learning_temporal_test.go`, `store/store_test.go`,
  `main_test.go` — claim tests, file-guard tests, race regression,
  re-ingestion seam test.
- `README.md` — one paragraph on the concurrent-supersession warning
  in the "Temporal Validity" subsection (there is no `store_learning`
  section).
- `AGENTS.md` — the Store Learning API `supersedes` bullet gains the
  concurrent-lost-claim warning cause and the write-time enforcement
  of "each has at most one successor"; the Recent Changes section
  gains an entry.
- `server.go` / `types/tools.go` — the `store_learning` tool
  `Description` and the `Supersedes` jsonschema description gain the
  concurrent-lost-claim cause, matching the
  learning-temporal-validity documentation precedent.
- No MCP tool schema changes, no CLI changes, no configuration changes,
  no SQLite schema changes. The only new observable is the
  `supersedes_warning` value on a call that would previously have
  forked the chain silently.

## Constitution Alignment

Assessed against the Unbound Force org constitution.

### I. Composability First

**Assessment**: PASS

The fix is entirely internal to Dewey: a store-layer SQL predicate and
tools-layer guards. No new dependencies (`json_extract` is SQLite JSON1,
already used in-tree via `modernc.org/sqlite`), no coupling to any other
Unbound Force tool. `dewey serve` on any vault keeps working unchanged.

### II. Autonomous Collaboration

**Assessment**: PASS

All behavior stays inside the existing `store_learning` MCP tool
contract. The conflict outcome rides the existing `supersedes_warning`
result field — no new tool, no new schema, no runtime coupling. Agents
need no knowledge of Dewey internals to interpret the warning.

### III. Observable Quality

**Assessment**: PASS

The failure mode becomes observable: a losing writer's result carries
`supersedes_warning` naming the winning successor identity, where today
the fork is silent. The store and the markdown backup — the auditable
artifacts in `.uf/dewey/` — are kept consistent, and the divergence
the guards detect is surfaced rather than masked.

### IV. Testability

**Assessment**: PASS

Every piece is testable in isolation with no external services: the
store claim against in-memory SQLite (`:memory:`), the file guard
against `t.TempDir()`, the race regression with goroutines and a start
barrier (deterministic outcome — exactly one claim can land — so no
sleeps), and the re-ingestion seam with a fresh store. All new tests
run under `-race -count=1` per the repo's testing conventions.
