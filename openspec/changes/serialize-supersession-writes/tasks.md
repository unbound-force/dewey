## 1. Store claim (CAS)

- [x] 1.1 `store/store.go`: guarded claim method (e.g.
  `ClaimSupersession(pageName, propertiesJSON string) (claimed bool,
  winner string, err error)`) — `UPDATE pages SET properties = ?,
  updated_at = ? WHERE name = ? AND source_id = ? AND (properties IS
  NULL OR properties = '' OR COALESCE(json_extract(properties,
  '$.superseded_by'), '') = '')` with `source_id` bound to
  `store.SourceLearning` and a rows-affected check; zero rows
  disambiguated by `GetPage` (missing page, non-learning `source_id`,
  or non-string `superseded_by` via comma-ok → error; present →
  decode and return the recorded successor); `updated_at` in Unix
  milliseconds; parameterized only; GoDoc citing the
  `PromotePagesTiers` precedent
- [x] 1.2 `store/store_test.go`: table-driven claim tests
  (`:memory:` SQLite) — claim lands; second claim refuses with the
  winner identity; missing page errors; non-learning `source_id`
  errors; non-string `superseded_by` (number, boolean, object, array)
  errors instead of yielding an empty winner; present-but-undecodable
  properties on the disambiguating re-read error; empty-string, `NULL`,
  JSON-`null` (missing key), and `{}` properties pass the guard;
  malformed non-empty properties fail closed; `updated_at` advances in
  Unix milliseconds; other columns (tier, content hash) untouched
- [x] 1.3 Phase 1 checkpoint — `go test -race -count=1 ./store/...`
  passes before phase 2 begins

## 2. Tools wiring — store leg

- [x] 2.1 `tools/learning_temporal.go`: supersession store leg inside
  a caller-level critical section owned by a small
  `withLearningPropsLock(fn)` helper that holds `learningPropsMu`
  across both legs (the acquisition moves out of
  `updateLearningProperties`, which becomes lock-free and is only
  called inside such a section) — re-read, in-closure `superseded_by`
  fast-fail that routes through the shared lost-claim outcome carrying
  the recorded winner (the `markStaleUnverified` precedent, but never
  a generic store-leg error), mutate, marshal, then the guarded claim
  instead of `UpdatePage`; lint's `markStaleUnverified` store path
  keeps its guard under the same restructured hold
- [x] 2.2 `tools/learning_temporal.go`: `linkSuperseded` returns a
  `supersedes_warning` naming the winning successor on a lost claim,
  whether the loss came from the CAS or the in-closure fast-fail; the
  losing learning is kept with its `supersedes` claim; the file leg is
  skipped when the claim does not land
- [x] 2.3 `tools/learning_temporal_test.go`: lost-claim path (target
  superseded after validation) — warning names the winner, store value
  untouched, file untouched; the in-closure fast-fail names the winner
  rather than a generic error; present-but-undecodable properties on
  the disambiguating re-read surface the store-leg failure warning;
  existing partial-failure and sequential-validation tests still pass
- [x] 2.4 Phase 2 checkpoint — `go test -race -count=1 ./tools/...`
  passes before phase 3 begins

## 3. Tools wiring — file leg

- [x] 3.1 `tools/learning_temporal.go`: pure helper
  `frontmatterScalar(content, key) (value string, found bool)` —
  top-level line scan beside `setFrontmatterKeys`; bare,
  double-quoted, and single-quoted scalars decoded to value
  semantics (`%q`-style escapes, `''` doubling); empty value =
  unclaimed; unparseable value returned verbatim so the caller treats
  it as claimed; no error return (the caller owns the fail-closed
  decision); tests for each form
- [x] 3.2 `tools/learning_temporal.go`: guarded backup write — read,
  normalize the recorded value and the identity with
  `supersedesIdentity`, check `superseded_by` (refuse on a differing
  non-empty or unparseable value, proceed when absent/empty/equal),
  then `setFrontmatterKeys` + write; run within the caller-level
  `learningPropsMu` hold that also covers the store leg, for both
  `updateLearningBackup` callers (`linkSuperseded`, and
  `markStaleUnverified` in `tools/lint_anchors.go` — the file leg
  refuses to write when the backup already records `superseded_by` or
  `validity: superseded`, leaving lint's observable outcomes unchanged
  and the lock scope widened to span store + file)
- [x] 3.3 `tools/learning_temporal_test.go`: file-guard tests —
  differing value refuses and leaves the file byte-identical with the
  refusal reported; unparseable scalar refuses; equal value proceeds
  and repairs a missing `validity`; a recorded `learning/<identity>`
  equal to the bare identity proceeds; absent/empty proceeds; missing
  file is a no-op; unsafe identity error unchanged
- [x] 3.4 Phase 3 checkpoint — `go test -race -count=1 ./tools/...`
  passes before phase 4 begins

## 4. Concurrency and durability

- [x] 4.1 `tools/learning_temporal_test.go`: race regression
  (SSW-FR-001, scenario "Two concurrent supersessions of the same
  target") — N goroutines (8+) released by a closed-channel start
  barrier all call `StoreLearning` superseding the same target;
  exactly one result carries no `supersedes_warning`, the rest name
  the winner; store and backup `superseded_by` equal the winner; all
  N successors stored with `supersedes` intact; no sleeps; passes
  `-race -count=1`
- [x] 4.2 `main_test.go`: re-ingestion seam (SSW-FR-004, scenario
  "Chain survives graph.db loss"; write→read round-trip per
  determinism pack DE-007) — after a supersession whose file leg wrote
  the backup, a fresh store plus `reIngestLearnings` restores exactly
  one `superseded_by` (the winner) with `validity: superseded` and the
  winner's `supersedes`; chains the real tools writer output into the
  real main parser
- [x] 4.3 regression guard (LTV-FR-001, scenario "Unknown or
  already-superseded target"): superseding an already-superseded
  target sequentially still fails validation with the existing
  "already superseded by" error before any write
- [x] 4.4 Phase 4 checkpoint — `go test -race -count=1 ./...` passes
  before phase 5 begins

## 5. Docs and assessment

_Checkpoint: all three assessment tasks complete; the code paths they
audit are already green from phase 4._

- [x] 5.1 [P] Docs: README "Temporal Validity" subsection (under
  "File-Backed Learnings" — there is no `store_learning` section)
  gains one paragraph on the concurrent-supersession
  `supersedes_warning`; AGENTS.md Store Learning API `supersedes`
  bullet names the new concurrent-lost-claim warning cause and the
  write-time enforcement of "each has at most one successor", and the
  AGENTS.md Recent Changes section gains an entry; the
  `store_learning` tool `Description` in `server.go` and the
  `Supersedes` jsonschema description in `types/tools.go` gain the
  concurrent-lost-claim cause; GoDoc on every new symbol (claim
  method, helpers), and rewritten contracts on the existing unexported
  `learningPropsMu` and `updateLearningProperties`, whose
  lock-ownership contract changes
- [x] 5.2 [P] Website docs-sync assessment per the AGENTS.md gate:
  internal concurrency fix with no command/flag/tool-input/config
  change. At design time `unbound-force/website` issue #380 (documents
  the temporal-validity surface) is open; confirm it still exists and
  is open at implementation time, then extend it with a scoped
  comment, or file a fresh issue if the warning semantics warrant one.
  Record the issue number (or the exemption rationale) inline in this
  task entry (distinct from 5.3's `completion.md` target)
  Result: website#380 verified OPEN and extended with a scoped
  docs-sync comment (issuecomment-6784758201); blog determination
  recorded on website#392, also OPEN (issuecomment-6784758479) —
  below threshold for a standalone post. No new issue needed.
- [x] 5.3 [P] Constitution alignment verification: confirm the four
  principles hold in the implemented code — no new dependencies and no
  coupling (I), warning rides the existing MCP result field (II), the
  fork is surfaced with named provenance instead of silent (III),
  deterministic sleep-free tests under `-race` (IV); record the check
  in the change's completion notes (`completion.md`)

## 6. Verification and close-out

- [x] 6.1 CI parity gate — replicate exactly the commands
  `.github/workflows/ci.yml` declares (the workflow file is the single
  source of truth; never copy a memorized command list): expect
  `go build ./...`, `go vet ./...`, `go test -race -count=1
  -coverprofile=coverage.out ./...`, and the Gaze `crap` invocation.
  Derive the Gaze thresholds (`--max-crapload`, `--max-gaze-crapload`)
  from the workflow at run time rather than hardcoding values; any
  failure blocks task completion
- [x] 6.2 `/uf.review-council` (OpenCode) until APPROVE, then draft
  the PR (closes #166 via `originating_issue`)
- [x] 6.3 Phase 6 checkpoint — CI parity green and review-council
  APPROVE recorded in the change's completion notes
