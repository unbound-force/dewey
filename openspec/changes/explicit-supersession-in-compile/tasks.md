<!--
  Task markers: `[P]` marks a task safe to run in parallel with the
  other `[P]` tasks in its group — it touches different files, depends
  on no prior task in the group, and needs no ordering. Unmarked tasks
  run sequentially first; `[P]` tasks run after them.
-->

## 1. Compile input: temporal metadata

- [ ] 1.1 `tools/compile.go`: `LearningEntry` gains `Validity`,
  `Supersedes`, `SupersededBy`; `buildLearningEntries` decodes them
  from page properties with the #146 keys via
  `decodeLearningProperties`, then normalizes and shape-validates at
  the decode boundary — trim strings, normalize the two link values
  with `supersedesIdentity`, accept `validity` only from the known
  token set, require link values to match the identity grammar, and
  treat a failing value as absent so it is never rendered into a
  prompt (ES-FR-003); GoDoc on the struct fields
- [ ] 1.2 `buildSynthesisPrompt`: render per-learning temporal lines
  (validity, supersedes, superseded-by) only when present — legacy
  learnings render unchanged
- [ ] 1.3 `tools/compile_test.go`: entry decoding (present, absent,
  wrong-typed values), decode-boundary withholding (non-string link,
  unknown validity token, non-identity link, surrounding whitespace
  normalized), and prompt rendering tests
- [ ] 1.4 Phase checkpoint — `go test -race -count=1 ./tools/...`
  passes before phase 2 begins

## 2. Prompt policy: declared authority, open conflicts, citations

- [ ] 2.1 `buildSynthesisPrompt` + `categoryInstructions`: remove the
  recency instructions ("favoring the newest learning", "Newer
  decisions supersede older ones", "superseded by a newer learning");
  decision and context rules restate declared-supersession authority
- [ ] 2.2 prompt gains the `## Open Conflicts` instruction: both
  positions presented, each with citations, no silent winner
- [ ] 2.3 prompt gains the per-claim citation instruction (bracketed,
  comma-separated identities per D3, with the identity-grammar shape
  shown and non-identity bracketed text stated as ignored) and the
  strict `## Supersession Proposals` section spec (successor,
  target, rationale; clear replacement only — recency is not grounds)
- [ ] 2.4 `tools/compile_test.go`: update prompt assertions — the
  "temporal merge instruction for decisions" pin (~line 706) becomes
  a declared-supersession assertion; add Open Conflicts, citation,
  and proposals-section assertions
- [ ] 2.5 Phase checkpoint — `go test -race -count=1 ./tools/...`
  passes before phase 3 begins

## 3. Citation coverage helper (pure)

- [ ] 3.1 `tools/citations.go` (new): claim-bearing body extraction
  (frontmatter, `## History`, `## Supersession Proposals` excluded),
  bracketed-citation extraction gated on the identity-grammar
  discriminator (whole-group match; every other bracketed form
  ignored), forward/backward coverage result type with named
  violations and counts, and the superseded-source backward rule (a
  superseded declared source is accounted for by the History table or
  an in-body mention; a current source is not); pure functions,
  GoDoc, Options/Result pattern
- [ ] 3.2 `tools/citations_test.go` (new): table-driven — clean
  article, truncated citation, unknown identity, uncited source,
  multi-identity group, History/frontmatter/proposals excluded from
  forward counting, superseded source accounted for by History,
  current source with only a History row still reported uncited, and
  one case per non-citation bracket form (`[text](url)`,
  `[ref][target]`, `[1]`, `[x]`, `[[wiki-link]]`, `[]`,
  `[see the notes]`)
- [ ] 3.3 Phase checkpoint — `go test -race -count=1 ./tools/...`
  passes before phase 4 begins

## 4. Compile: proposals and coverage

- [ ] 4.1 `tools/compile.go`: parse, validate, and strip the `##
  Supersession Proposals` section from synthesized output — one pure
  helper shared with `store_compiled`; entries valid when successor
  and target resolve to the cluster's source learnings, are distinct,
  and carry non-empty rationale; invalid entries dropped and reported
- [ ] 4.2 compile result: `proposed_supersessions` (successor,
  target, rationale, topic) and per-article `coverage` in each
  `articles` entry; `prompts_ready` result shape otherwise unchanged
- [ ] 4.3 `tools/compile_test.go`: canned synthesizer outputs —
  valid proposals parsed/stripped/surfaced; absent section; invalid
  entries dropped with the article still written; per-article
  coverage present and correct, including a superseded source
  accounted for by the generated History table
- [ ] 4.4 Phase checkpoint — `go test -race -count=1 ./tools/...`
  passes before phase 5 begins

## 5. store_compiled: input validation and parity

- [ ] 5.1 `tools/compile.go` `StoreCompiled`: declared sources must
  map to existing learning pages (#93) — error result naming the
  unresolved sources, no page or file written; coverage verified and
  reported; `## Supersession Proposals` parsed, stripped, and
  returned via the shared helper
- [ ] 5.2 `tools/compile_test.go`: unknown-source refusal (nothing
  written), coverage and proposals parity with the compile path
- [ ] 5.3 Phase checkpoint — `go test -race -count=1 ./tools/...`
  passes before phase 6 begins

## 6. supersede_learning tool

- [ ] 6.1 `tools/learning_temporal.go`: helper recording `supersedes`
  on an existing successor — store properties plus backup frontmatter
  via `updateLearningBackup`, designed to run inside a
  `withLearningPropsLock` hold; GoDoc
- [ ] 6.2 `tools/supersede.go` (new): `SupersedeLearning` handler on
  the `Learning` struct — resolve successor and target (identity or
  `learning/<identity>`, `supersedesIdentity` normalization);
  validation ladder before any write (existence, distinct, target
  records no `superseded_by`, successor declares no differing
  `supersedes`, bounded cycle walk with a visited set and hop bound
  over declared links in both directions); then re-read and
  re-validate both sides' link state inside one
  `withLearningPropsLock` hold (successor fast-fail mirrored from
  `claimSuperseded`); then the successor leg followed by the guarded
  target claim (CAS + backup leg);
  `supersedes_warning` on lost claim or partial failure
- [ ] 6.3 `types/tools.go`: `SupersedeLearningInput` with jsonschema
  descriptions; `server.go`: registration and tool description (tool
  count 50 → 51); `server_test.go`: extend `TestNewServer_ToolCount`
  ranges and per-category comments
- [ ] 6.4 `tools/supersede_test.go` (new): confirmation path (store +
  backup frontmatter on both sides, target still present with its
  tier unchanged), unknown identity, same learning,
  already-superseded target, successor-declares-other, cycle, cycle
  walk terminating on a pre-existing cyclic chain that excludes the
  requested pair, lost claim warning naming the winner, partial-
  failure warning, concurrent same-target race (start barrier, no
  sleeps, `-race`), and a concurrent reverse-pair race (A→B and B→A
  started together: exactly one link lands, the other is refused or
  warns, and no cycle is written)
- [ ] 6.5 `tools/learning_temporal_test.go` + `main_test.go`:
  tool-created links round-trip through `reIngestLearnings` — the
  successor's `supersedes` and the target's `superseded_by` restored
  after store loss
- [ ] 6.6 Phase checkpoint — `go test -race -count=1 ./...` passes
  before phase 7 begins

## 7. lint: citation_coverage and remediation text

- [ ] 7.1 `tools/lint.go`: `citation_coverage` check over compiled
  pages (content from store blocks, sources from page properties) via
  the shared helper; findings name unresolved citations and uncited
  sources; summary counters; `fix` never touches them
- [ ] 7.2 `tools/lint.go` contradiction remediation text and the
  stale "temporal merge" doc comments in `lint.go`/`compile.go`
  corrected to the declared-supersession workflow
  (`supersede_learning` + compile proposals)
- [ ] 7.3 `tools/lint_test.go`: coverage findings for hand-edited and
  `store_compiled`-authored articles, clean article passes, a
  pre-change article reported (intended signal) and not cleared by
  `fix`, counters in the summary, remediation-text assertion
- [ ] 7.4 Phase checkpoint — `go test -race -count=1 ./tools/...`
  passes before phase 8 begins

## 8. /dewey-distill review gate

- [ ] 8.1 `.opencode/commands/dewey-distill.md`: compile-result
  inspection reports `proposed_supersessions` and per-article
  coverage; the review gate presents them beside the promote list;
  the `HUMAN_GATE` question asks which proposals to confirm alongside
  the promote selection (marker stays reserved for the two gates — no
  third marked question); confirmed pairs applied via
  `supersede_learning` before `promote`; unconfirmed proposals left
  unchanged; a confirmed apply that errors or warns aborts promotion
  and relays the failure verbatim; the report section lists applied
  supersessions
- [ ] 8.2 `slash_commands.go`: update the embedded `dewey-distill.md`
  copy byte-identically (drift-tested by
  `TestInitCmd_ScaffoldsDeweyDistillFromMap` in `cli_test.go`)
- [ ] 8.3 Phase checkpoint — `go test -race -count=1 ./...` passes
  (the drift test is the gate for 8.2) before phase 9 begins

## 9. Docs, website, and verification

- [ ] 9.1 [P] `README.md`: compile citations, coverage, and
  proposals; `supersede_learning` row and tool count 50 → 51; lint
  `citation_coverage` kind; distill review gate; and correct the two
  falsified prose claims — line ~431 and the compile section at
  line ~757 both say compiled articles "resolve temporal
  contradictions — newer facts replace older ones" — to the
  declared-supersession policy
- [ ] 9.2 [P] `AGENTS.md`: MCP tool count (overview, `server.go`
  architecture note, `types/tools.go` note), tool table/summary
  entry, Recent Changes, and annotate the insert-time-only
  `store_learning` `supersedes` bullet (line ~461, which reads as the
  only way to link learnings) with the `supersede_learning`
  two-existing-learnings path
- [ ] 9.3 [P] Website docs-sync: determine which
  `content/docs/` pages describe compile contradiction resolution,
  citations, lint kinds, or the tool count, then file one
  `unbound-force/website` docs issue and record its number inline here
- [ ] 9.4 [P] Website blog-sync: determine whether a release-style
  post is warranted for the policy change, then file the blog issue
  (or record the deliberate no-issue determination with its reason)
  and record the number inline here
- [ ] 9.5 [P] Website tutorial-sync: determine the impact on the
  distill tutorial (published as website#376) and any compile
  walkthrough, then file the tutorial issue and record its number
  inline here
- [ ] 9.6 `specs/013-knowledge-compile/contracts/compile-tool.md`
  and `contracts/lint-tool.md`: annotate the superseded newer-wins
  wording (the compile tool description's "decisions: temporal
  merge" and the lint contradiction check) with a pointer to this
  change's declaration that it supersedes 013 FR-008, so the archived
  contract and the new policy cannot be read as agreeing
- [ ] 9.7 Constitution-alignment verification: re-read the four
  principles against the final diff (Composability First — no new
  dependency, store-less mode untouched; Autonomous Collaboration —
  proposals/coverage ride the existing results, one MCP tool;
  Observable Quality — coverage and proposals machine-parseable;
  Testability — pure helpers, in-memory store, `NoopSynthesizer`),
  and record any drift as a finding before marking the phase done
- [ ] 9.8 CI parity: derive the exact commands from
  `.github/workflows/ci.yml` at run time (build, vet,
  `go test -race -count=1 -coverprofile=coverage.out ./...`, Gaze
  `crap` with the workflow's thresholds) and run them locally; any
  failure blocks task completion
- [ ] 9.9 `/uf.review-council` (OpenCode) until APPROVE, then draft
  the PR (closes #150 via `originating_issue`)
