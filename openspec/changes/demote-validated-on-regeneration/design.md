## Context

`Store.UpdatePage` currently preserves `validated` unconditionally (`CASE WHEN tier = 'validated' THEN 'validated' ELSE ? END`, PR 145). `tools/compile.go:s​ynthesizeClusters` carries the previous `validated` tier onto regenerated content, and `clearCompiled`/`removeStaleTopics` retain validated pages whose tag disappeared. The net effect is that regenerated — human-unreviewed — content keeps a human vouch.

The narrow intent of the PR 145 guard was to close a read-then-write race around `promote` (a metadata write racing promotion must not demote). The guard as written also protects content-changing writes, which was never intended and becomes load-bearing for unattended distill (#193, epic #153 Phase 5). #203 (git-is-source-of-record) will make the same behavior fall out from blob identity; this change is the store-level approximation so the pipeline can run unattended before #203.

Constraints: no schema migration (`content_hash` already exists on `pages`), no new dependencies or CGO, the 37 inherited graphthulhu tools keep their contracts, and every rule must stay testable in isolation with `:memory:` SQLite and `NoopSynthesizer`/`scriptedSynth`.

## Goals / Non-Goals

### Goals
- Make `validated` vouch for specific content: preserved only when `content_hash` is unchanged.
- Remove the compile-time `validated` carry-over; preservation becomes the store rule's job (byte-identical → `validated` stays, changed → `draft`).
- Give `compile`/`store_compiled` pages a meaningful `content_hash` (truncated SHA-256 over the persisted bytes) so the store rule can distinguish changed vs unchanged.
- Stop `curate` re-index from carrying `validated` onto changed knowledge files; unchanged files are skipped pre-write.
- Preserve the promotion race fix and the retention carve-outs (failed-topic, total-failure, empty-collection keep validated).

### Non-Goals
- Stale-topic and empty-collection retention semantics beyond the content-binding rule (unchanged content keeps its vouch; flagged for #203/#193).
- Git-is-source-of-record (#203) general mechanism.
- Changing `store_compiled`'s validated-overwrite refusal or disk-vault/external-source re-index observable semantics except via the store rule.

## Decisions

### 1. Content-bound `UpdatePage` — hash equality, not timestamps
**Decision**: `tier = CASE WHEN tier = 'validated' AND ? = content_hash THEN 'validated' ELSE ? END` — three bound params: `content_hash` for the equality check, `properties`/`content_hash`/`is_journal` unchanged, `tier` as the else branch. Two empty hashes compare equal (so hash-less writers keep pre-change behavior). Non-validated tiers take the caller-supplied value unconditionally.

**Rationale**: Single-SQL demotion avoids a read-then-write race that a Go-level `if` would reintroduce. `content_hash` is already indexed and queryable; using equality rather than wall-clock time makes the rule deterministic and audit-friendly (tier + hash jointly define provenance).

**Alternatives**: Application-level demotion before `UpdatePage` — rejected because it would require callers to re-implement the promote-race guard and could be bypassed. Adding a separate `validated_content_hash` column — rejected (schema change, redundant).

**Constitution**: *Observable Quality* — `content_hash` becomes the auditable witness for the tier; III PASS in proposal. *Testability* — rule is exercised directly via `store/store_test.go:TestUpdatePage_*`; IV PASS.

### 2. Compile always writes `draft`; store decides preservation
**Decision**: Delete the `if existing != nil && existing.Tier == "validated" { tier = "validated" }` branch in `synthesizeClusters` and the `ListPages` lookup it required. Every synthesized cluster gets `tier = "draft"`. `persistCompiledArticle` is called with that tier, the `content_hash` computed over the article bytes, and the same bytes written to `.uf/dewey/compiled/{tag}.md`. Byte-identical regeneration still preserves `validated` because `UpdatePage` now does so on hash equality; changed bytes demote.

**Rationale**: Separates concerns — compile is a pure producer of new content (Composability First, II PASS), store owns the trust boundary. Removes a store read from the hot path and a behavior that cannot be tested without a prior validated row.

**Alternatives**: Keep the lookup and add a hash check inside compile — rejected (duplicates store logic, violates single place for tier policy).

### 3. Content hashes for compiled pages — truncated SHA-256
**Decision**: `contentHash(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:8]) }` applied in `persistCompiledArticle` (compile) and in `StoreCompiled`'s page construction, over exactly the bytes persisted to `pages` and written to disk. Reuses the curation re-index / learning re-ingestion convention (first 8 bytes hex, 16-char string).

**Rationale**: Matches existing convention, fits `content_hash TEXT` without migration, collision risk negligible for change-detection. Hashing the persisted bytes (not the pre-wrap prompt) ensures the store comparison is self-consistent.

**Constitution**: *Composability First* — no new dependency; *Observable Quality* — hash now queryable for compiled pages.

### 4. Curate re-index — write `curated` on change, skip on equality
**Decision**: In `tools/curate.go:IndexKnowledgeStore`, when `existing.Tier == "validated"` and hashes differ, set `existing.Tier = "curated"` (not `validated`). When hashes equal, return early without `UpdatePage`. New pages keep `tier: curated` as before.

**Rationale**: Mirrors epic rule "unattended curation never writes `validated`". Unchanged validated knowledge pages incur no write, so their vouch is untouched without special-casing.

**Alternatives**: Reuse the content-bound store rule for curate as well — considered but curate's tier on change is `curated`, not `draft`, so the store rule would need to know the caller intent; simpler to set the correct tier at the call-site and rely on the equality-skip.

### 5. Promote exclusivity — no machine path writes `validated`
**Decision**: No code change to `tools/promote.go`; its `UPDATE ... WHERE tier = 'draft'` stays the sole promotion path. This change asserts (VTB-FR-005) that none of `compile`/`store_compiled`/`curate` emit `validated`.

### 6. Edge handling — empty hash, UTF-8 truncation, stale paths
- Empty-hash equality preserves validated for legacy rows / hash-less writers; once a compiled/curated page has a non-empty hash, subsequent writes are compared normally.
- `content_hash` is computed over raw article bytes; no UTF-8 truncation is needed (hash input is binary-safe). Existing UTF-8 truncation tests in `store` remain unaffected.
- `clearCompiled`/`removeStaleTopics` paths are untouched; they delete only non-validated rows, so retention of validated empty-collection/failure pages remains governed by dewey-distill as required.

## Risks / Trade-offs

- **Hash-less validated pages never demote** until first hash-bearing write updates their hash. Accepted: callers that never supply a hash intentionally retain the old never-demote semantics; the first compiled/curated write after this change backfills the hash and flips to content-bound behavior.
- **Truncated hash collision** (8 bytes) could keep `validated` on changed content with probability 2^-64. Accepted: far below operational risk; full SHA-256 would be a storage change for no measurable benefit.
- **Compile lookup-failure test removal**: `TestSynthesizeClusters_LookupFailureReturnsError` loses its target code. The lookup branch is deleted, so the test is deleted with it; the underlying store failure mode is still covered by `store` tests and by compile's other error paths. The preserve-validated test is repurposed to assert demotion (`draft`) while keeping provenance (`compiled_by`) assertions.
- **No cross-store transaction**: compile's per-tag writes remain independent; a partial synthesize failure still retains prior validated articles (existing `retainedValidated` logic). Full `validated` preservation across a failed batch is intentionally not attempted — it would require a multi-row transaction outside the current store API.

## Constitution Alignment

From proposal (§ Constitution Alignment): I Autonomous Collaboration — PASS (artifact-level trust boundary, no new coupling), II Composability First — PASS (internal to `store`/`tools`, no new deps or config), III Observable Quality — PASS (tier + hash are queryable provenance), IV Testability — PASS (all seams exercised via `:memory:` SQLite / `NoopSynthesizer`). Design preserves each: no runtime coordination is added, `compile` result still reports `tier`/`compiled_by` per article, and every scenario maps to an existing test seam.

<!-- scaffolded by uf vdev -->
