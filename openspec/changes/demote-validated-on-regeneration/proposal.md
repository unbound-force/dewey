## Why

`Store.UpdatePage` never demotes a `validated` page: the tier column
is written through a `CASE WHEN tier = 'validated' THEN 'validated'
ELSE ? END` guard (`store/store.go`, introduced with the PR 145
promotion trust boundary). On top of that store guard, `compile`'s
synthesis loop explicitly carries the old article's `validated` tier
onto regenerated content (`tools/compile.go`, `synthesizeClusters`),
and its stale cleanup retains validated pages whose tag is gone.

The net effect: when `compile` regenerates the article for a tag
whose page is `validated`, machine-written content that no human has
reviewed keeps the `validated` tier a human gave to the *old*
content. The store guard exists to close a read-then-write race
around `promote` — a write that does not change content must not
revert a promotion — but as written it also protects writes that
*do* change content, which was never the intent.

That is harmless while a human runs every compile. It stops being
harmless once distill runs unattended (#193): every weekly run
would silently carry "reviewed" onto unreviewed rewrites, and the
epic #153 rule "automate demotion, not promotion" would be inverted
in practice — the machine would be perpetuating promotions instead
of flagging them for re-review.

Tracked in #204 (epic #153 "Epistemic integrity for Dewey
knowledge", Phase 5; related: #203 git-is-source-of-record, #193
unattended distill). #203 makes this behavior fall out naturally
(the regenerated blob no longer matches the merged, vouched blob);
if #203 lands first this change reduces to a test. This change is
the targeted store-level fix so the unattended pipeline cannot ship
before #203 does.

## What Changes

- **Store rule becomes content-bound**: `UpdatePage` preserves a
  stored `validated` tier only when the incoming content hash equals
  the stored hash. When the hash changes, the caller-supplied tier
  applies. Writes that do not change content — the promote race the
  existing comment and test describe — still cannot demote a
  validated page.
- **`compile` stops carrying `validated` onto regenerated
  articles**: every article `compile` produces is written with tier
  `draft`; the store rule decides preservation (byte-identical
  regeneration keeps `validated`, changed content drops to `draft`).
  The per-topic tier lookup in `synthesizeClusters` is removed along
  with the behavior it served.
- **Machine write paths record content hashes**: `compile`
  (`persistCompiledArticle`) and `store_compiled` set the page's
  `content_hash` from the stored article bytes. Compiled pages
  currently carry an empty hash, which would make the store rule
  inert for them; hashing the written bytes makes changed-vs-
  unchanged detectable and tightens the accepted millisecond
  promote-race window in `store_compiled`'s overwrite refusal.
- **`curate` re-index stops preserving `validated` on content
  change**: a knowledge file whose content changed re-indexes at its
  machine tier (`curated`), the same tier a fresh insert gets.
  Unchanged files are skipped before any write, so an untouched
  validated knowledge page keeps its tier. This honors the epic rule
  "unattended curation never writes a validated tier".
- **Test updates**: the compile test asserting that a recompile
  preserves `validated` (`TestCompile_PreservesValidatedTierAnd_
  Provenance`) is repurposed to assert demotion plus unchanged
  provenance recording; the `synthesizeClusters` lookup-failure test
  is removed with the lookup it exercised.

Out of scope, deliberately:

- **Stale-topic and empty-collection retention**
  (`removeStaleTopics`, `clearCompiled`) keeps validated articles
  whose tag is gone. Their content is unchanged — still the bytes a
  human reviewed — so under the content-binding rule the vouch
  remains intact, and the dewey-distill requirement "validated
  knowledge is never silently destroyed by an empty collection"
  still governs. Revisit under #203/#193 if drift should flag them.
- **#203 git-is-source-of-record** — the general mechanism this
  change approximates at the store layer. This change is
  independently shippable and is subsumed by #203.
- **`store_compiled`'s validated-overwrite refusal** — unchanged;
  it still refuses before any write. Only its residual race window
  improves, as a side effect of the hash now being recorded.
- **Disk-vault and external-source re-index paths** — analyzed in
  the design; their observable semantics beyond the store rule are
  unchanged.

## Capabilities

### New Capabilities
- `validated-tier-content-binding`: a `validated` tier vouches for
  specific content. The store preserves `validated` only across
  writes that do not change content (hash equality); machine
  regeneration paths (`compile`, `store_compiled`, `curate` re-index)
  record content hashes and never carry `validated` onto changed
  content.

### Modified Capabilities
- `knowledge-distillation`: the "Provenance and tier preservation"
  scenario of the Server-Side Synthesis Wiring requirement —
  "recompiled article keeps tier `validated`" — flips to
  demote-on-regeneration: a recompile with changed content lands at
  `draft`; only byte-identical regeneration keeps `validated`.
  Provenance recording (`compiled_by`) is unchanged. The
  failed-topic, total-failure, and empty-collection retention
  scenarios are untouched (no write occurs, so no demotion occurs).

### Removed Capabilities
- None.

## Impact

- **`store/store.go`** — `UpdatePage` SQL and GoDoc: the tier CASE
  gains a content-hash equality condition; one additional bound
  parameter.
- **`tools/compile.go`** — `synthesizeClusters` (tier lookup
  removed, always draft), `persistCompiledArticle` (content hash),
  GoDoc/comment updates.
- **`tools/compile.go` (`StoreCompiled`)** — content hash on the
  persisted page.
- **`tools/curate.go`** — knowledge-file re-index tier carry
  removed; comment updated.
- **Tests** — `store/store_test.go`, `tools/compile_test.go`,
  `tools/curate_test.go`.
- **User-visible behavior** — the `compile` result's per-article
  `tier` field and the regenerated article's frontmatter now report
  `draft` where they previously reported `validated`; regenerated
  validated articles re-enter the promote queue. A website docs
  sync issue is required (website repo, `docs` label).
- **No schema change** — `content_hash` already exists on `pages`;
  compiled rows start carrying a value where they carried an empty
  string.

## Constitution Alignment

Assessed against the Unbound Force org constitution.

### I. Autonomous Collaboration

**Assessment**: PASS

The change tightens an artifact-level trust boundary rather than
adding coupling: tiers remain store-attached provenance metadata on
pages, machine paths keep producing self-describing results (the
`compile` result still reports `tier` and `compiled_by` per
article), and no new runtime coordination between components is
introduced. Demotion is deterministic store behavior, not a
cross-component negotiation.

### II. Composability First

**Assessment**: PASS

Dewey remains independently installable and usable alone. The fix
is internal to the existing `store` and `tools` packages, adds no
dependencies, and requires no configuration. Degraded (store-less)
modes are untouched — the rule lives in the persistence layer that
optional persistence already defines.

### III. Observable Quality

**Assessment**: PASS

The demotion is observable and auditable: the page's `tier` column,
the compile result's per-article `tier` field, and the article
frontmatter all reflect the landing tier, and `content_hash` —
already a first-class, queryable column — becomes meaningful for
compiled pages. The change strengthens provenance: a `validated`
badge now reliably means "a human vouched for these bytes", which is
the metadata contract review agents consume.

### IV. Testability

**Assessment**: PASS

Every rule lands at a seam that is testable in isolation with
in-memory SQLite (`:memory:`) and the existing `NoopSynthesizer` /
`scriptedSynth` test doubles: the store CASE is exercised through
`UpdatePage` directly, the compile path through `Compile` end to
end, and the curate path through its re-index helper. The
acceptance criteria map one-to-one onto deterministic tests,
including the existing race-regression test, which must continue to
pass unmodified.
