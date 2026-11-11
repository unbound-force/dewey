## Context

`store_learning`'s supersession flow spans a seconds-wide window
between validation and write. `prepareTemporal` resolves the target
and enforces the one-successor invariant ("chains, not forks",
learning-temporal-validity design D3) by reading `superseded_by` from
the store (`resolveSupersedesTarget`). The successor is then written —
markdown file (`O_CREATE|O_EXCL`), `InsertPage`, `PersistBlocks`,
`GenerateEmbeddings` — before `linkSuperseded` updates the target in
two legs:

1. **Store leg** — `updateLearningProperties` under `learningPropsMu`
   re-reads the page and mutates its properties, but
   `linkSuperseded`'s mutate closure sets `superseded_by`
   unconditionally; the final `store.UpdatePage` writes the whole row.
   The mutex (added with #147 so lint's fix and supersession cannot
   drop each other's property writes) serializes the read-modify-write
   in-process — it does not re-check the invariant, and it cannot see
   writers that bypass it. `markStaleUnverified` in
   `tools/lint_anchors.go` already re-checks its validity guard inside
   the mutate; the supersession closure is the one place that does
   not.
2. **File leg** — `markSupersededFile` →
   `updateLearningBackup` does an unguarded read-modify-write on the
   target's markdown frontmatter
   (`setFrontmatterKeys` replaces keys in place), with no lock at all.

The store's process lock serializes *processes*; the race is concurrent
in-process MCP tool calls under streamable HTTP sharing one store
handle. Two concurrent callers superseding the same target both pass
validation; the second store leg overwrites the first successor, and
the interleaved file legs can leave store and file naming different
successors. No `supersedes_warning` is emitted. Because
`reIngestLearnings` (main.go) restores only pages missing from the
store, the markdown file is the durable truth after `graph.db` loss —
a stale file leg silently reintroduces the fork.

The issue (#166) constrains the fix: every write leg must be guarded,
and the PR-145 toctou pattern applies — CAS preferred over lock
widening. The in-tree precedent is `Store.PromotePagesTiers`
(`store/store.go`): a conditional `UPDATE ... WHERE name = ? AND tier
= 'draft'` plus a rows-affected check that refuses on zero rows,
putting the guard inside the store operation.

## Goals / Non-Goals

### Goals

- Exactly one successor survives concurrent same-target supersession,
  in the store and in the markdown backup.
- The losing call reports `supersedes_warning` naming the winning
  successor; its learning is kept (existing D4 semantics from
  learning-temporal-validity).
- The recorded chain survives `graph.db` deletion plus re-ingestion.
- Deterministic, sleep-free tests that pass under `-race`.
- Sequential supersession behavior is byte-for-byte unchanged.

### Non-Goals

- Healing pre-existing forks (files that already record a forked
  `superseded_by` are surfaced by the guard, not rewritten).
- Cross-process serialization of markdown writes beyond the existing
  store lock (processes that hold the store are serialized by the
  flock; a process writing the learnings directory without the store
  is unchanged territory).
- General properties-blob concurrency for keys other than
  `superseded_by` (the `learningPropsMu` boundary stays as #147 drew
  it).
- Changing validation, MCP schemas, CLI, or the SQLite schema.
- Closing the store-first crash window: a process that dies after the
  store claim lands but before the file write leaves the store ahead
  of the backup. SSW-FR-004 covers only supersessions whose file leg
  wrote, so the markdown record can lag until a later write repairs
  it.

## Decisions

### D1 Store leg: compare-and-swap in the store op (PR-145 pattern)

A new `store` method claims the supersession atomically. It writes
only the `properties` and `updated_at` columns — never the whole
row — guarded on the target recording no successor and on the row
still being a learning:

```sql
UPDATE pages SET properties = ?, updated_at = ?
WHERE name = ?
  AND source_id = ?
  AND (properties IS NULL
       OR properties = ''
       OR COALESCE(json_extract(properties, '$.superseded_by'), '')
          = '')
```

with a rows-affected check. `source_id` is bound to
`store.SourceLearning`, re-asserting row identity the way PR-145's
`tier = 'draft'` predicate does, so a same-named page that was
concurrently replaced cannot be stamped. `updated_at` is written in
Unix milliseconds, matching every other writer in the store.

`json_extract` (SQLite JSON1) is already used in-tree by
`GetPagesWithProperty`; no new dependency, no schema migration —
`superseded_by` stays in the properties JSON per
learning-temporal-validity D1. The `properties IS NULL OR properties =
''` disjuncts mirror `GetPagesWithProperty`'s guard (`store/store.go`):
`json_extract` raises a malformed-JSON error on an empty-string
document rather than returning NULL, so `COALESCE` alone cannot absorb
it. With the disjuncts, a `properties` value of `NULL`, `''`, JSON
`null`, or a missing key is unclaimed, matching
`decodeLearningProperties`'s semantics (empty text decodes to an empty
map). Malformed non-empty properties JSON still makes `json_extract`
error, so the claim fails closed into the existing store-leg failure
warning.

Why CAS over the per-target mutex the issue also allows: a per-target
lock registry needs creation, cleanup, and its own guard map, and it
only defeats writers that know to take it. The SQL predicate defeats
every writer to the row, including future ones that bypass
`tools`-package locking — the rule from the PR-145 postmortem: for
every check-then-act, verify each writer either holds the same lock or
is defeated by an atomic guard on the write itself.

### D2 Claim outcome taxonomy, composed with the existing mutex

The store method returns three distinguishable outcomes:
`(claimed, winner, error)` — claimed when the row updated;
not-claimed with the recorded successor's identity when the guard
refused (disambiguated by a follow-up `GetPage`: a missing page, a
page whose `SourceID` is not `store.SourceLearning`, or a page whose
`superseded_by` is not a string, is a hard error, keeping today's
"failed to mark ... in the store" warning); and `error` for query
failures. A present page whose properties cannot be decoded on the
disambiguating re-read is also a hard error, surfaced through the same
store-leg failure warning — the fail-closed precedent of
`resolveSupersedesTarget`'s "has unreadable properties" error
(`tools/learning_temporal.go`). The recorded value is read with a
comma-ok type assertion, so a non-string `superseded_by` (number,
boolean, object, array) takes this hard-error path instead of
surfacing as an empty winner. On a refused claim the target's
properties are untouched.

The tools layer keeps its read-modify-write under `learningPropsMu`
(read page → decode → mutate → marshal) so in-process writers
preserving *other* property keys (lint fix) cannot be dropped, and the
mutate closure gains the same in-closure `superseded_by` fast-fail
that `markStaleUnverified` already uses for `validity` — but the SQL
guard is the authority. The fast-fail MUST NOT return its generic
store-leg error: when it detects a recorded `superseded_by`, it routes
through the same lost-claim outcome as a zero-row CAS, carrying the
recorded winner so the caller emits the SSW-FR-002
`supersedes_warning` naming it. Belt and suspenders: the mutex covers
known in-process writers; the predicate covers everything else. The
validation-time page snapshot passed to `linkSuperseded` is used for
the target's identity only, never for its properties.

A lost claim produces `supersedes_warning` naming the winner (for
example: `learning "X" was concurrently superseded by "Y"; this
learning's supersedes claim is dangling`). The losing learning is not
rolled back — the successor is already stored, matching D4 of
learning-temporal-validity. The file leg does not run when the claim
does not land.

### D3 File leg: guarded replace under the same lock

Only the claim winner reaches `markSupersededFile`. The file leg
becomes read → check → write:

1. Read the backup (missing file stays a no-op, as today).
2. Extract the top-level `superseded_by` value with a new pure helper
   beside `setFrontmatterKeys` (line-based, same conventions):
   `frontmatterScalar(content, key) (value string, found bool)` — no
   error return. A missing key or empty value is unclaimed. Bare,
   double-quoted, and single-quoted scalars are recognized and decoded
   to value semantics (`%q`-style escapes in double quotes, `''`
   doubling in single quotes), because the writer emits `%q`-escaped
   values while re-ingestion parses full YAML; an identity needing
   escaping therefore compares correctly instead of being permanently
   refused. An unparseable value is returned verbatim so the caller
   treats it as claimed-and-differing — fail closed rather than
   clobber.
3. Normalize the recorded value and the identity being written with
   `supersedesIdentity`, then refuse when the normalized recorded value
   names a successor other than the one being written; leave the file
   byte-for-byte unchanged and return the refusal as the warning.
   Proceed when unclaimed or equal (idempotent re-write also repairs a
   missing `validity` key).
4. Otherwise `setFrontmatterKeys` + write, unchanged.

Each caller performs its store leg and file leg under a single
`learningPropsMu` hold: the acquisition moves out of
`updateLearningProperties` into a caller-level critical section
(`linkSuperseded` and `markStaleUnverified`). To keep the invariant
enforceable rather than merely documented, a small
`withLearningPropsLock(fn)` helper owns the mutex; both callers wrap
their two legs in it, and `updateLearningProperties` becomes lock-free
and is only called from inside such a section. Its GoDoc and
`learningPropsMu`'s comment are rewritten to state the
caller-holds-the-lock contract. No exported API changes.

Per-leg locking would not close the stale-knowledge divergence:
`markStaleUnverified`'s store leg (whose in-closure guard re-checks
`validity`) and its file leg would be separate acquisitions, so a
supersession could land both of its legs between them and lint's late
file leg could overwrite `validity: superseded` with `stale-unverified`
in the durable backup while the store keeps `superseded`. Holding the
lock across both legs closes that interleaving; both callers and both
legs are the mutex's only clients, so the restructuring is contained.
Since only the claim winner writes the file, two supersession file
legs cannot contend for the same target; the mutex closes the
remaining interleaving (supersession file leg vs lint fix file leg on
the same learning), and the value check closes cross-leg divergence
(store unclaimed, file already marked — pre-fix residue).

For `markStaleUnverified`, the file-leg guard compares the backup's
`superseded_by` alongside its store-leg `validity` re-check: when the
backup records a `superseded_by` or `validity: superseded`, the file
leg refuses to write and leaves the backup unchanged. In the normal
path lint's store leg skips the learning first, because it now records
`validity: superseded`; the file-leg refusal therefore only fires on
pre-fix residue (a backup that already carries a successor while the
store still reads `current`), and it never downgrades the durable
record. Lint's observable outcomes are otherwise unchanged.

Refuse-and-warn was chosen over self-healing toward the store's claim:
the file is the durable record after `graph.db` loss, so silently
rewriting a `superseded_by` it already carries could mask a genuine
fork; a warning names both successors and leaves the operator in
control. The issue's phrasing ("re-read and re-check before
replacing") prescribes refusal on mismatch.

Store-first ordering is kept: the store is the validation authority,
and a file write without a landed claim would mark the durable record
for a supersession that validation no longer honors. A failed file leg
after a landed claim remains a warning (existing semantics).

### D4 Test strategy (Testability)

- **Store claim** (`store/store_test.go`, in-memory SQLite,
  table-driven): claim lands once; a second claim on the same target
  refuses and reports the winner; a missing page, a non-learning
  `source_id`, and a non-string `superseded_by` each error; a present
  page with undecodable properties on the disambiguating re-read is a
  hard error; empty-string, `NULL`, JSON-`null` (missing key), and
  `{}` properties pass the guard; malformed non-empty properties fail
  closed; `updated_at` advances in Unix milliseconds; other columns
  (tier, content hash) untouched.
- **Tools guards** (`tools/learning_temporal_test.go`): the
  validation-passed-then-lost-claim path returns a warning naming the
  winner and leaves the store untouched; the in-closure fast-fail
  likewise names the winner rather than a generic error; the file
  guard refuses a differing value (file byte-identical), including the
  unparseable-scalar case, proceeds on absent/empty/equal and on a
  prefixed `learning/<identity>` record equal to the bare identity,
  and treats a missing file as a no-op.
- **Race regression** (SSW-FR-001; scenario "Two concurrent
  supersessions of the same target"): N goroutines released by a
  closed-channel start barrier all call `StoreLearning` superseding
  the same target; assert exactly one result carries no
  `supersedes_warning`, the other N−1 carry one naming the winner; the
  store's `superseded_by` and the backup's frontmatter both equal the
  winner's identity; all N successor pages exist with their
  `supersedes` intact. No sleeps — the CAS makes "exactly one winner"
  deterministic under any interleaving. Must pass `-race`.
- **Re-ingestion seam** (SSW-FR-004; scenario "Chain survives
  graph.db loss"; write→read round-trip per determinism pack DE-007):
  after a supersession whose file leg wrote the backup, build a fresh
  store and run `reIngestLearnings` — the restored target carries
  exactly the winner as `superseded_by` with `validity: superseded`,
  and the winner carries `supersedes`. This chains the real tools
  writer's output into main's real parser.
- **Sequential regression** (LTV-FR-001; scenario "Unknown or
  already-superseded target"): superseding an already-superseded
  target still fails validation with the existing error before any
  write.

### D5 Documentation

The only user-visible delta is a new `supersedes_warning` reason. The
supersession prose in `README.md` lives in the "Temporal Validity"
subsection under "File-Backed Learnings" — there is no
`store_learning` section — so that subsection gains one paragraph on
concurrent supersession. `AGENTS.md`'s Store Learning API `supersedes`
bullet gains the new concurrent-lost-claim warning cause and notes
that "each has at most one successor" is now write-time enforced, and
the AGENTS.md Recent Changes section gains an entry.

The in-product MCP surfaces change too: the `store_learning` tool
`Description` in `server.go` and the `Supersedes` jsonschema
description in `types/tools.go` both gain the concurrent-lost-claim
cause, matching learning-temporal-validity's documentation precedent.
GoDoc on the new store method and helpers, plus rewritten contracts on
the existing unexported `learningPropsMu` and
`updateLearningProperties`.

The website docs-sync assessment happens at implementation time: this
is an internal concurrency fix with no command/flag/tool-input/config
change, so the gate is likely exempt, but the assessment is recorded
per the AGENTS.md gate. Blog determination: below threshold — a
bug-fix hardening of an existing feature is not a standalone story, so
the outcome is recorded as a comment on the open temporal-validity
blog issue (website#392) rather than as a new post.

## Risks / Trade-offs

- **Properties-blob granularity.** The CAS guards `superseded_by`
  only; a concurrent non-mutex writer changing *other* keys in the
  same JSON blob between the tools-layer read and the claim can still
  be lost. Pre-existing boundary, unchanged: the mutex covers known
  in-process writers.
- **Pre-existing forks are surfaced, not healed.** A backup already
  naming a different successor than a freshly landed claim keeps its
  value with a warning. Deliberate (D3) — the durable record is never
  silently rewritten.
- **Store-first crash window.** A process that dies between the landed
  store claim and the file write leaves the store ahead of the backup.
  The markdown record lags until a later write repairs it, and
  SSW-FR-004's durability claim is scoped to supersessions whose file
  leg wrote. Named as a non-goal rather than closed.
- **Widened mutex hold time.** Each caller now holds
  `learningPropsMu` across its store leg and its file leg. Bounded (one
  small row update plus one small file write), and only
  learning-property writers contend.
- **`updated_at` churn.** Each claim bumps `updated_at`; harmless for
  lint's staleness heuristics (learnings are `draft` tier, not
  knowledge-store sources).
- **Line-based frontmatter reading.** The guard reads what
  `setFrontmatterKeys` writes; hand-authored YAML outside the
  recognized scalar forms fails closed into a warning rather than a
  silent overwrite. The seam test keeps the writer and the
  re-ingestion parser honest.
