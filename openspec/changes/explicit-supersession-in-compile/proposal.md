<!-- originating_issue: 150 -->

## Why

`compile` resolves contradictions by telling synthesis to favor the
newest learning — `tools/compile.go`, `buildSynthesisPrompt`:
"Resolve contradictions by favoring the newest learning", and
`categoryInstructions`: "Newer decisions supersede older ones on the
same topic". Recency is not evidence: a newer learning can be
wrong, partial, or about a different branch, and the compiled
article silently presents the winner's claims as current with no
trace of the conflict. This supersedes 013-knowledge-compile
FR-008's newer-wins temporal merge ("decision learnings get
temporal merge (newer wins …)"); declared supersession, not
recency, becomes the replacement authority.

#146 gave learnings the machinery to say precisely which learning
replaces which — two-way `supersedes` / `superseded_by` links,
`validity`, and the write-time-enforced one-successor claim from
#166 — but compile never reads those fields: `LearningEntry`
carries identity, tag, category, date, and content only. The one
place that should use temporal validity to decide what is current
ignores it, and the one place a human could confirm a correction
mid-pipeline (the `/dewey-distill` review gate) has no tool that can
link two *existing* learnings — `store_learning` only links at
insert time, to a freshly created successor.

Provenance has the same gap. A compiled article cites its sources
as one whole-article frontmatter list; no claim in the body traces
to a learning, and nothing checks the two directions that matter:
that every claim cites a real source (forward), and that every
source learning is accounted for (backward). The KB effort's
forward/backward coverage check caught 35 truncated citations and a
missed batch of ~119 source files that a one-directional check
would not have — both are failure modes dewey's compile can
currently produce silently.

Part of #153. Depends on #146 (temporal-validity fields
`supersedes` / `superseded_by`, merged) and is sequenced after #145
(server-side synthesis in `tools/compile.go` via `WithSynthesizer`,
merged). Related: #93 (validate `store_compiled` sources map to
existing learnings). Supersession proposals are recorded only on
human confirmation, so the review gate — not the model — remains
the authority that links two learnings.

## What Changes

- **The synthesis prompt stops resolving by recency.** A learning
  is treated as replaced only when a declared supersession link says
  so (`supersedes` / `superseded_by` / `validity: superseded`,
  surfaced per learning in the prompt). Contradictions without a
  declared link are presented in an `## Open Conflicts` section with
  both positions cited — surfaced, as `lint` contradictions already
  are, not silently resolved. This retires 013-knowledge-compile
  FR-008's newer-wins merge for the compile pipeline; the
  `specs/013-knowledge-compile/contracts/compile-tool.md` and
  `contracts/lint-tool.md` contracts are annotated with the
  superseded wording so the older contract cannot be read as
  agreeing with the new policy.
- **Compile reads temporal metadata.** `LearningEntry` gains
  `validity`, `supersedes`, and `superseded_by`, parsed from page
  properties, normalized and shape-validated at the decode
  boundary, and the prompt renders them per learning.
- **Claims cite their sources.** The prompt requires every factual
  claim to end with a bracketed citation of the learning identity
  (or identities) it derives from, so an article's provenance is
  checkable claim by claim.
- **Compile proposes; a human confirms.** Synthesis may append a
  strict `## Supersession Proposals` section (successor, target,
  rationale) only when one learning clearly replaces another —
  recency alone is not grounds. Compile parses, validates, and
  strips it; the proposals ride the compile result to the
  `/dewey-distill` review gate, and a confirmed proposal is
  recorded by a new `supersede_learning` MCP tool that links two
  existing learnings through the guarded write paths #146/#166
  built (CAS store claim, guarded backup leg,
  `withLearningPropsLock`, in-closure link-state re-validation,
  plus a bounded cycle check the fresh-successor path never needed).
- **Forward/backward coverage check, both homes.** Compile verifies
  each article it writes — forward: every citation-shaped token
  resolves to a source learning; backward: every source learning is
  accounted for, with superseded sources accounted for by the
  History table or an in-body mention — and reports per-article
  coverage in its result. `lint` gains a `citation_coverage` kind
  that re-runs the same check over already-written articles
  (hand-edited or `store_compiled`-authored) for drift. The check is
  one shared pure helper.
- **`store_compiled` validates its inputs (#93).** Declared sources
  must map to existing learnings (error); citation coverage is
  verified and reported like compile's.
- **`/dewey-distill` closes the loop.** The review gate presents
  proposed supersessions and coverage findings next to the promote
  list; confirmed supersessions are applied via `supersede_learning`
  before promotion, and a failed apply aborts promotion rather than
  promoting unlinked articles. Stale "resolve via temporal merge"
  remediation text in `lint` and compile doc comments is corrected
  to match.

## Capabilities

### New Capabilities

- `explicit-supersession`: compile-time contradiction policy —
  declared supersession is the only authority, unconfirmed
  conflicts are surfaced, claims carry per-claim citations,
  coverage is checked forward and backward at compile and lint
  time, and human-confirmed supersessions between existing
  learnings are recorded through a guarded tool. Requirements
  ES-FR-001 .. ES-FR-013.

### Modified Capabilities

- `knowledge-distillation`: the `/dewey-distill` review gate
  presents proposed supersessions and coverage and applies
  confirmed pairs before promotion, aborting promotion when a
  confirmed apply fails. The capability has no archived main spec
  yet, so its delta is expressed as one ADDED requirement
  (KD-FR-104) alongside the pending "Gated Distill Workflow"
  contract from the `dewey-distill` change. KD-FR-104 explicitly
  supersedes that change's "Prompt-Mode Fallback Compatibility"
  byte-compat clause for compile prompt payloads; payload shape
  stays compatible, prompt text does not.

### Removed Capabilities

- None.

## Tier Exception (Human-Approved)

This change stays an OpenSpec change although its artifact set is
thicker than the tactical shape usually implies. The exception is
human-approved through the `/uf.run-pipeline-plan` pause-relay — the
same relay that carried this change's spec-review decisions, which
are recorded in the review-council dispatch history
(`.uf/artifacts/dispatch/`, correlation ids preserved in the
checkpoints) and applied to these artifacts — for two
reasons. First, the change is a thin guarded wrapper over store
machinery that already exists — #146's temporal fields, #166's
serialized supersession claim, and #145's server-side synthesis —
not a new capability surface with its own lifecycle. Second, the
artifacts already carry speckit-grade rigor: 14 numbered
requirements with scenarios, eight numbered design decisions, and
a nine-phase task list. Escalating to a Speckit feature directory
would rename the same content without adding review value, and
would discard the spec-review decisions already recorded against
these artifacts. The tier decision is recorded here so a later
reader does not read the artifact depth as an accident.

## Impact

- `tools/compile.go` — `buildSynthesisPrompt` /
  `categoryInstructions` rewrite; `LearningEntry` temporal fields;
  decode-boundary normalization and shape validation; proposal
  parsing, validation, and stripping; per-article coverage in the
  result; doc comments.
- `tools/citations.go` (new) — pure citation extraction (identity
  grammar discriminator) and forward/backward coverage helpers
  shared by compile, `store_compiled`, and lint.
- `tools/supersede.go` (new) — `supersede_learning` handler;
  reuses `tools/learning_temporal.go` guarded legs; adds the
  successor-side `supersedes` write, in-closure link-state
  re-validation, and a bounded cycle guard.
- `tools/lint.go` — `citation_coverage` check; contradiction
  remediation text.
- `tools/learning_temporal.go` — helper for recording `supersedes`
  on an existing successor (store properties + backup frontmatter).
- `types/tools.go` — `SupersedeLearningInput`.
- `server.go` — `supersede_learning` registration inside the
  existing `if !readOnly` block, beside the other mutating tools
  (tool count 50 → 51); the `compile` MCP tool description drops
  "resolves contradictions temporally" for the declared-supersession
  policy.
- `cli.go` — `dewey compile` Long help ("resolves contradictions
  temporally (newer wins)" → the declared-supersession policy) and
  `dewey lint` Long help (enumerates `citation_coverage`).
- `.opencode/commands/dewey-distill.md` — review gate presents
  proposals and coverage; confirmed supersessions applied before
  promote; apply failure aborts promotion.
- `slash_commands.go` — embedded `dewey-distill.md` copy updated
  byte-identically (drift-tested by `cli_test.go`); the embedded
  `dewey-compile.md` copy updated byte-identically with its tracked
  file (both must change together).
- `.opencode/commands/dewey-compile.md` — description drops
  "resolves contradictions temporally" for the declared-supersession
  policy.
- `.opencode/agents/curator.md` — `supersede_learning` added to the
  tool allowlist (human-approved extension of the protected agent
  tool-access surface, AGENTS.md Gatekeeping Value Protection item
  5: the gate's confirmed-apply step requires it); `agents.go`
  `deweyAgentDefinitions["curator.md"]` embed synced byte-identically
  and the `cli_test.go` allowlist set-equality assertions extended —
  all three surfaces move together.
- `.uf/dewey/manifest.md` — regenerated (`dewey manifest`) so the
  compile entry's stale recency text does not survive.
- `specs/013-knowledge-compile/contracts/compile-tool.md` and
  `contracts/lint-tool.md` — annotated with the superseded
  newer-wins wording so the archived contract and this delta do
  not read as agreeing; the authoritative
  `specs/013-knowledge-compile/spec.md` restatements of FR-008
  annotated with the same pointer — FR-008 itself, acceptance
  criterion 4, the user-story prose, the independent test,
  acceptance scenario 2, SC-002, and SC-007 — plus the
  `research.md` R7 and `tasks.md` T022 restatements.
- Tests: `tools/compile_test.go`, `tools/citations_test.go` (new),
  `tools/supersede_test.go` (new), `tools/lint_test.go`,
  `tools/learning_temporal_test.go`, `main_test.go`,
  `server_test.go` (tool-count ranges).
- `README.md` — compile citations/coverage, `supersede_learning`
  row, lint kind, distill gate; the two "resolve temporal
  contradictions — newer facts replace older ones" claims corrected.
- `AGENTS.md` — MCP tool count, tool table/summary, the
  insert-time-only `supersedes` bullet annotated with the
  `supersede_learning` path, Recent Changes, and the enumerations
  this change invalidates: the CLI Commands `dewey lint` row (gains
  `citation_coverage`), the Project Overview category list, and the
  architecture `tools/` line (gains the supersede entry).
- Website sync issues in `unbound-force/website` (docs, blog, and
  the distill tutorial, one consolidated determination recorded in
  tasks.md) — scoped against the adjacent published surfaces:
  blog #392 and tutorial #393 document supersession as
  insert-time-only, and the lint-kind comparator precedent is blog
  #399 / tutorial #402.
- No SQLite schema changes, no configuration changes, no new
  dependencies. New observables: `proposed_supersessions` and
  per-article `coverage` in compile results, the
  `citation_coverage` lint kind, and the `supersede_learning` tool.

## Constitution Alignment

Assessed against the Unbound Force org constitution.

### I. Autonomous Collaboration

**Assessment**: PASS

The change moves conflict policy out of a prompt's private judgment
and into artifacts agents can act on: proposals and coverage ride
the existing `compile` result, confirmations go through one MCP
tool, and `/dewey-distill` keeps every decision behind its existing
human gates. No runtime coupling is introduced — the
`supersede_learning` tool is self-describing and the guarded write
paths are unchanged in shape.

### II. Composability First

**Assessment**: PASS

Dewey remains independently installable and unchanged for anyone
who never compiles: keyword tools, the 37 inherited graphthulhu
tools, and store-less mode are untouched. No new dependency; the
citation check is pure string analysis; the linking tool reuses
existing store methods.

### III. Observable Quality

**Assessment**: PASS

Silent newest-wins resolution becomes observable: conflicts are
stated in the article with citations, proposals carry
successor/target/rationale, and both directions of citation
coverage are machine-parseable results (compile report + lint
findings) rather than prose. Provenance is checkable per claim,
with truncated citations and uncited sources surfaced by name.

### IV. Testability

**Assessment**: PASS

Every piece is testable in isolation without external services:
prompt construction and proposal parsing are pure functions,
coverage checking is pure over article text plus a source list,
`supersede_learning` runs against in-memory SQLite and
`t.TempDir()` backups, and the compile path uses `NoopSynthesizer`
canned output — all under `-race -count=1`.
