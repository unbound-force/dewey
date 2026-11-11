## Context

Compile resolves contradictions by telling the synthesizer to prefer
recency. `buildSynthesisPrompt` (`tools/compile.go`) instructs
"Resolve contradictions by favoring the newest learning", and
`categoryInstructions` encodes newest-wins per category — "Newer
decisions supersede older ones on the same topic", context carried
forward "unless explicitly superseded by a newer learning". Recency
is not evidence: a newer learning can be wrong, partial, or about a
different branch, and the winner's claims land in the article as
current fact with no trace of the conflict. `compile_test.go` (~line
706) pins the current wording.

The machinery to say which learning replaces which exists but is
ignored. #146 gave learnings `validity`, `supersedes`, and
`superseded_by` (page properties plus markdown frontmatter,
round-tripped through re-ingestion), and #166 serialized the
same-target supersession claim (`store.ClaimSupersession` CAS store
leg, guarded backup leg via `updateLearningBackup`, both under
`withLearningPropsLock`). But compile's `LearningEntry` carries
identity, tag, category, date, and content only — the one place that
should use temporal validity to decide what is current never reads
it.

Provenance has the same shape of gap. `buildCompiledArticle` writes a
whole-article `sources` frontmatter list and an auto-generated
`## History` table; no claim in the body traces to a learning, and
nothing checks either direction — that every citation resolves
(forward) and that every source is cited (backward). The KB effort's
forward/backward coverage check caught 35 truncated citations and a
missed batch of ~119 source files that a one-directional check would
not — both failure modes dewey's compile can currently produce
silently.

Around that core: `store_compiled` (`tools/compile.go`) accepts
arbitrary `sources` without checking they exist (#93); no tool links
two *existing* learnings — `store_learning` records `supersedes` only
at insert time, to a freshly created successor, so the
`/dewey-distill` review gate could not confirm a correction
mid-pipeline even if it saw one; and lint's contradiction finding
still tells the user to "resolve contradictions via temporal merge"
(`tools/lint.go`, `checkContradictions` remediation), which this
change makes false.

The `/dewey-distill` command (`.opencode/commands/dewey-distill.md`)
is embedded byte-identically in `slash_commands.go` and drift-tested
by `TestInitCmd_ScaffoldsDeweyDistillFromMap` (`cli_test.go`) — both
copies must change together.

## Goals / Non-Goals

### Goals

- The synthesis prompt treats a learning as replaced only when a
  declared supersession link or validity says so; unconfirmed
  contradictions surface in an `## Open Conflicts` section with both
  positions cited.
- Compile reads temporal metadata (`LearningEntry` gains `validity`,
  `supersedes`, `superseded_by`) and renders it per learning.
- Every factual claim carries a bracketed citation of its source
  learning identity or identities.
- Synthesis may append a strict `## Supersession Proposals` section;
  compile and `store_compiled` parse, validate, and strip it, and the
  proposals ride the compile/store result to the review gate.
- A new `supersede_learning` MCP tool links two existing learnings
  through the #146/#166 guarded write paths, plus a cycle check.
- One shared pure helper checks citation coverage forward and
  backward; compile reports per-article coverage, lint gains a
  `citation_coverage` kind, and `store_compiled` validates its
  declared sources (#93).
- `/dewey-distill` presents proposals and coverage at the review gate
  and applies confirmed supersessions before promotion.

### Non-Goals

- Auto-confirming supersessions — every link is human-confirmed at
  the review gate; nothing is written on the proposal's own
  authority.
- Changing `store_learning` supersession semantics, the CAS guard, or
  the losing-writer outcome from #166.
- Blocking article writes on coverage violations — coverage is
  reported (compile result, lint findings) for the human gate to
  judge, never a write failure.
- Extending coverage to knowledge-store curated articles (they
  declare no learning sources).
- SQLite schema changes, configuration changes, new dependencies, or
  changes to the 37 inherited graphthulhu tools and store-less mode.
- Reworking clustering — tags remain the sole clustering dimension.

## Decisions

### D1 Prompt policy: declared links only, conflicts surfaced

`buildSynthesisPrompt` and `categoryInstructions` drop every recency
instruction. The decision rule becomes: a decision is replaced only
when a declared supersession link or `validity: superseded` says so;
otherwise both positions appear in an `## Open Conflicts` section,
each with its citations. The context rule drops "superseded by a
newer learning" for the same declared-link phrasing. Conflicts are
surfaced the way lint surfaces contradictions — stated, not resolved
— because the resolution authority is the declared link, which only
a human can add between two existing learnings (Observable Quality:
the conflict becomes visible in the article instead of silently
resolved).

This retires 013-knowledge-compile FR-008's newer-wins temporal
merge for the compile pipeline. The two archived contracts that
restate that rule — `specs/013-knowledge-compile/contracts/
compile-tool.md` (the tool description's "decisions: temporal
merge") and `contracts/lint-tool.md` (the contradiction check) —
are annotated in place so a reader of the older contract is told
which delta supersedes it, rather than finding two documents that
silently disagree.

### D2 Temporal fields on `LearningEntry`

`buildLearningEntries` decodes `validity`, `supersedes`, and
`superseded_by` from page properties (the same keys #146 writes,
decoded with the shared `decodeLearningProperties` helper) into
`LearningEntry`, and `buildSynthesisPrompt` renders a validity line
plus supersedes/superseded-by lines per learning, only when present.
Legacy learnings render unchanged, so existing clusters produce
byte-identical prompts except for the new instructions.

Decoding is a trust boundary, not a pass-through: a page property is
content that may have been written by another agent or a web source,
and the prompt is a place where raw text changes model behaviour. So
the decode step normalizes and shape-validates before rendering:
string values are trimmed, the two link values are normalized with
`supersedesIdentity`, `validity` is accepted only from the known
token set, and each link value must match the identity grammar.
Anything that fails — a non-string value, an unknown validity token,
a link that is not an identity — is treated as absent and never
rendered, so a hostile or corrupt property cannot inject prompt
text (ES-FR-003).

### D3 Citation grammar: one bracketed group, comma-separated

A citation is a square-bracketed group holding one or more
comma-separated learning identities, for example
`[auth-20261101T000000-alice]` or
`[auth-20261001T000000-alice, auth-20261101T000000-bob]`. The prompt
shows the form; the extractor parses exactly this shape. One grammar,
shared by the prompt, the coverage helper, and the tests, keeps the
check deterministic (Testability).

The discriminator is shape matching against the identity grammar — a
tag, an `YYYYMMDDTHHMMSS` timestamp, and an author — not "anything in
brackets". A bracketed group counts as a citation only when every
comma-separated element, trimmed, matches that grammar; any other
group is not a citation and is ignored by both directions. That is
what keeps ordinary markdown from reading as unresolved citations:
link text `[the docs](…)`, reference labels `[ref][target]`,
footnotes `[1]`, checkboxes `[x]`, wikilinks `[[…]]`, empty `[]`,
and prose `[see the notes]` all fail the grammar and are ignored.
A truncated identity such as `[auth-20261101T000000-al]` still
matches (author `al`), so it is extracted and reported unresolved —
the failure mode the KB effort actually hit. A group that misspells
an identity so badly that no element matches is dropped rather than
reported forward; the backward direction still catches it, because
the source it should have cited then appears uncited.

### D4 Supersession Proposals: strict section, parsed and stripped at
both write paths

The prompt specifies a trailing section in exactly this shape:

```
## Supersession Proposals

- successor: <identity>
  target: <identity>
  rationale: <single line>
```

and states the justification bar: one learning clearly replaces the
other; recency alone is not grounds. Parsing, validation (both
identities resolve to the article's source learnings, distinct,
non-empty rationale), and stripping live in one pure helper used by
both `synthesizeClusters`' write path and `store_compiled` — the
prompt-mode path (agent synthesis, `store_compiled` write) must
surface proposals too, or the review gate would only work with
server-side synthesis. Valid proposals ride the result as
`proposed_supersessions` (successor, target, rationale, topic);
invalid entries are dropped and reported without failing the article
write. Rationale is not persisted on the link — it rides the compile
result and the gate presentation; the link itself stays as lean as
#146's. (Autonomous Collaboration: proposals are artifacts riding
the existing compile result, not side-channel state.)

### D5 Coverage helper: pure, shared, three call sites

`tools/citations.go` (new) holds pure functions: extract the
claim-bearing body (frontmatter, `## History`, and any `##
Supersession Proposals` section excluded), extract bracketed
citations, and check both directions against the declared source
list, returning named violations plus counts. Compile calls it per
article (per-article `coverage` in each `articles` result entry);
`store_compiled` calls it on submitted content; lint's
`citation_coverage` check calls it over compiled pages (content from
store blocks, sources from page properties — the store is the source
of truth). Excluding History and frontmatter is what makes the
backward direction meaningful: the auto-generated table lists every
source by construction (Composability First: string analysis, no
embedder, no dependencies; Observable Quality: machine-parseable
violations, by name).

The backward rule needs one carve-out or it fights the temporal
model. A superseded learning should *not* appear as a current claim —
that is the whole point of supersession — so requiring an in-body
citation for it would demand the synthesis resurrect a retired
fact. The rule is therefore: a declared source that is not
superseded must be cited in the claim-bearing body; a declared
source that records `superseded_by` or `validity: superseded` is
accounted for when it is cited in the body *or* named in the `##
History` table or an in-body mention. The History table counts for
superseded sources only, so it cannot launder an uncited current
source into a pass. This keeps "every source learning is accounted
for" true without weakening "every current claim cites its source".

### D6 `supersede_learning`: two existing learnings through the
guarded paths

The handler lives in `tools/supersede.go` on the `Learning` struct
(reuses its store and vaultPath; no new wiring). Input is
`SupersedeLearningInput{successor, target}`, each an identity or
`learning/<identity>` normalized with `supersedesIdentity`. The
validation ladder refuses before any write: unknown identity, same
learning, target already records `superseded_by`, successor declares
a differing `supersedes`, or a cycle — following declared links from
either learning reaches the other. The cycle walk is the check the
fresh-successor path never needed (`store_learning`'s successor is
newly created, so it cannot close a loop). Because the walk follows
links that other writers may already have chained into a cycle, it
carries a visited set and an explicit hop bound: a pre-existing loop
the walk runs into terminates and refuses instead of spinning, and
the bound is what keeps a corrupt chain from turning a refusal into
a hang (ES-FR-008).

Validation happens before the lock, but validation-time state is a
snapshot. Both learnings already exist, so either side can change
between validation and write — unlike the fresh-successor path,
where only the target is reachable by another writer. The handler
therefore re-reads and re-validates each side's link state *inside*
the `withLearningPropsLock` hold, before the first write: the
in-closure fast-fail that `claimSuperseded` already uses for the
target (a recorded non-empty string routes to the lost-claim
outcome, a non-string is an error) is applied to the successor's
`supersedes` as well. A concurrent writer that linked either side
after validation is refused or reported as a lost claim rather than
silently overwritten (SSW-FR-001, SSW-FR-002).

Writes run under one `withLearningPropsLock` hold across both legs,
in `store_learning`'s order: the successor leg first (record
`supersedes` in store properties and backup frontmatter — a new
helper in `tools/learning_temporal.go`), then the guarded target
claim (CAS via `claimSuperseded`, backup via the guarded
`updateLearningBackup` leg). A lost claim or a file-leg failure
after the store leg lands reports `supersedes_warning`, matching
#166's losing-writer semantics; re-confirming an already-complete
link errors on the target-already-superseded guard, exactly as
`store_learning` does.
Registration in `server.go` takes the tool count 50 → 51;
`TestNewServer_ToolCount` ranges and per-category comments extend
with it. (Testability: in-memory SQLite plus `t.TempDir()` backups,
race-tested with a start barrier, no sleeps.)

### D7 `store_compiled` validates its inputs (#93)

After the existing scalar checks and before any write,
`store_compiled` resolves every declared source to an existing
learning page and returns an error result naming the unresolved
sources — no page, no file. Coverage is verified and reported like
compile's, and the proposals section is handled identically (D4).
Sources must exist; proposals naming non-source identities are
dropped and reported (not an error) — the article is still valid
data, only the proposal is not actionable.

### D8 Distill gate: one question, applied before promote

The review gate's single `HUMAN_GATE` question is extended to ask
which supersession proposals to confirm alongside the promote
selection — the marker stays reserved for the two existing gates
(curator-agent-human-gates constraint), so no third marked question
is introduced. After the answer, each confirmed pair is applied via
`supersede_learning` before `promote`; unconfirmed proposals leave
both learnings unchanged. Both copies of the command change together:
`.opencode/commands/dewey-distill.md` and the byte-identical
`slash_commands.go` entry.

## Risks / Trade-offs

- **Legacy articles flood the new lint kind — intended.** Compiled
  articles written before this change carry no per-claim citations,
  so every one of them produces a `citation_coverage` finding the
  first time lint runs. That is signal, not a false positive: the
  backlog names exactly the articles that predate the policy, and
  recompiling them (or correcting their citations) clears it.
  Records written by this change do not inherit it. Treating the
  initial wave as noise and suppressing the kind would hide the
  drift the kind exists to catch.
- **LLM citation compliance is probabilistic.** A model may omit or
  malformed citations. Mitigation: coverage is a reported finding at
  compile time and a lint kind afterward; violations name the exact
  token; the human gate judges. Writes are not blocked — an article
  with missing citations is still data, and blocking would make
  compile unusable with weaker models.
- **Strict section format may be mis-produced.** Invalid entries are
  dropped and reported, never silently kept and never failing the
  write; the section is always stripped, so stored articles stay
  clean either way.
- **compile.go complexity.** The new logic lands in pure helpers
  (`citations.go`, proposal parsing) with table tests; the Gaze CRAP
  gate is watched — the serialize-supersession-writes change needed a
  post-approval CRAP fix (its task 6.4), so complexity budgets are a
  known hazard here too.
- **Lock scope on `supersede_learning`.** The hold spans the
  successor and target legs — the same scope #166 drew for
  supersession (store + file under one hold), reusing the same mutex;
  no new lock ordering is introduced. Re-validation inside the hold
  means the lock is held across extra reads (both sides' pages), so a
  call costs two more store reads than a blind write; that is the
  price of not clobbering a concurrent reverse-pair link, and it is
  reads, not contention, because nothing blocks on the re-read except
  the same mutex every learning-property writer already takes.
- **Cycle-walk cost.** Chains are bounded by learnings per tag and
  walked via in-store reads before any write; the walk is refuse-only
  (no writes) and carries a visited set plus a hop bound, so a
  pre-existing cycle terminates and a long chain costs reads, not
  contention.
- **Tool-count surfaces.** 50 → 51 touches README, AGENTS.md, and the
  `server_test.go` ranges — tasked explicitly so the count cannot
  drift silently.

## Open Questions

Working defaults chosen for implementation; revisit only if
review raises a material objection.

- **OQ1 — Citation grammar details.** Default: one bracketed group
  with comma-separated identities (D3). Alternative: adjacent
  single-identity brackets (`[a][b]`). The default matches how the
  prompt reads naturally and parses with one grammar.
- **OQ2 — Persisting rationale on the link.** Default: no — the
  rationale rides the compile result and gate presentation; the link
  stays as lean as #146's `supersedes`/`superseded_by`. Alternative:
  a `supersession_rationale` property on the successor.
- **OQ3 — Coverage scope for lint.** Default: compiled articles only
  (they declare learning sources in properties). Knowledge-store
  curated articles declare none, so there is nothing to check
  backward against.
