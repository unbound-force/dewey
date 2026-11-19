# Design: Re-ingest Learnings When Their Files Change

## Context

See proposal. `reIngestLearnings` (main.go, package main) treats an
existing `learning/<identity>` page as proof the file is indexed, so an
in-place file edit — a pull, an overlay sync — never reaches the index
until `dewey reindex`. The knowledge-store path solved the same defect
in #220 (`knowledge-store-reindex`): `tools.IndexKnowledgeStore`
compares `ContentHash`, rebuilds blocks and embeddings only when
content changed, and refreshes tier and properties in place otherwise.
This design ports that shape to learnings without changing the
learning file format, the frontmatter, or the store schema.

## Goals / Non-Goals

### Goals

- A server restart after a pull converges every changed learning to
  its file: new body, new blocks, new embeddings, refreshed properties.
- Unchanged learnings cost no block or embedding work at startup — a
  file whose content hash hasn't changed is never re-embedded.
- Rows written by earlier versions (divergent or empty hashes) converge
  in one pass, then stay cheap forever.
- `store_learning` writes hashes that agree with the re-ingestion
  comparison, so tool-stored learnings never re-ingest spuriously.

### Non-Goals

- Runtime file watching of the learnings directory. Convergence is
  restart-based, exactly what issue #221 asks for ("a pull followed by
  a server restart would be enough").
- Pruning pages whose files were deleted, or whose frontmatter
  identity changed. Pre-existing behavior; `dewey reindex` clears both.
- Covering learnings from the `dewey index` / `dewey reindex` CLI
  commands. Learnings are restored at server startup, symmetric with
  knowledge stores (#220's non-goal).
- Changing the learning file format or frontmatter in any way.
- gh-hud dropping its `learnings changed` reindex nag — consumer-side
  work tracked in jflowers/gh-hud#138, not here.

## Decisions

### D1 Mirror `IndexKnowledgeStore`'s three-branch shape, in place

`reIngestLearnings`'s existing-page branch becomes the three branches
#220 established: page missing → insert (today's code, moved, not
rewritten); stored hash equals the file-derived hash → cheap path;
hash differs (including an empty stored hash) → full re-ingest via
`DeleteBlocksByPage` → `UpdatePage` → `PersistBlocks` →
`GenerateEmbeddings`. The hash computation moves above the
`GetPage` check so all three branches share it.

The mirror is at the pattern level, not shared code. The learning path
has its own frontmatter struct, identity validation, tier clamp and
properties derivation; the knowledge-store path parses document
frontmatter and derives tiers from git blobs. One shared function
would couple the two file-backed families' formats — the wrong DRY
level. Keeping it in package main also preserves the function's
parameter shape (store, embedder, vault path), which the existing
tests already exercise (Testability, constitution IV).

### D2 The comparison hash stays the file body

`sha256(body)[:8]`, hex — exactly what the insertion path computes
today (`body` is `splitFrontmatter`'s trimmed text after the second
`---`). Two reasons this MUST not change:

1. **Compatibility.** Knowledge stores hash the whole file; learnings
   hash the body. Switching learnings to whole-file hashing would
   invalidate every stored learning hash and re-embed every learning
   once at the next startup — precisely what the epic's cheap
   constraint forbids.
2. **Correct cheap-path routing.** Blocks derive from the body, so a
   frontmatter-only edit (`validity` flips written by
   `updateLearningBackup`, or arrived by pull) keeps the body hash
   stable and takes the cheap path — no block or embedding work for a
   change that cannot affect blocks.

### D3 `store_learning` hashes the trimmed information text

`store_learning` hashes raw `input.Information`
(tools/learning.go:297) while `writeLearningFile` writes
`information + "\n"` and re-ingestion hashes `TrimSpace` of the body.
For text without leading/trailing whitespace the two hashes are
identical; for padded text they diverge, and without alignment every
future padded learning would re-ingest once. The fix is one
expression: hash `strings.TrimSpace(input.Information)`. Nothing else
consumes the learning hash today, and no existing test pins its value.

Alternatives rejected: re-serializing the file body to match raw
information (changes the file format — forbidden), or comparing two
different hash inputs (impossible by construction). Existing divergent
rows converge once on the first startup after upgrade (accepted,
bounded — see Risks).

### D4 Refresh sets per branch

- **Cheap path (hash match):** refresh `Properties`, `Category` and
  `Tier` with one `UpdatePage` when any differs from the file-derived
  values — the backfill shape from #220's design D4. Tier is always
  the ingestion-derived `draft` (learnings clamp to draft on every
  route), so a row that drifted to a non-draft value is clamped back
  without block work.
- **Changed path (hash mismatch):** update the full file-derived row —
  `ContentHash`, `Properties`, `Tier` (`draft`, with the existing
  warning for forged non-draft claims), `Category`, and the recomputed
  `SourceID`/`SourceDocID` (same values the insert path writes) — then
  delete blocks, re-persist, re-embed. `DeleteBlocksByPage` cascades
  to embeddings through the schema's
  `embeddings.block_uuid REFERENCES blocks(uuid) ON DELETE CASCADE`,
  so stale embeddings cannot linger (no schema change, no migration).
- **`created_at` is immutable on both paths:** `UpdatePage`'s SQL does
  not touch it, and the value is frontmatter-stable (the identity
  embeds the timestamp). No action needed.

### D5 Count semantics and logging

The return value counts full re-ingests (inserts plus content
changes) only; cheap-path refreshes do not count — mirroring
`IndexKnowledgeStore`'s `indexed` counter. The existing startup log
line ("learnings re-ingested from files") therefore fires only when
real work happened, keeping cheap starts log-silent exactly as today
(Observable Quality, constitution III: the count stays truthful).

## Risks / Trade-offs

- **First-start convergence.** Rows whose stored hash disagrees with
  the file-derived hash — legacy rows written by `store_learning` over
  untrimmed text, or rows with an empty hash — re-ingest once after
  upgrade, regenerating their blocks and embeddings. Bounded and
  one-time; the file is the authority, so convergence is the correct
  outcome, not a regression.
- **Startup cost.** Unchanged learnings already pay a file read, a
  parse and a `GetPage` per file today; the change adds one SHA-256
  over the body and, occasionally, one row update. No new I/O shape.
- **Blocks from raw vs. trimmed text.** `store_learning` parses blocks
  from raw `input.Information` while re-ingestion parses from the
  trimmed body; for padded text the two differ. Pre-existing asymmetry
  (out of scope), and convergence makes the file the single authority
  after one pass.
- **Test updates for superseded pins.** Two existing tests pin the
  pre-change skip behavior and update with this change.
  `TestReIngestLearnings_SkipsExisting` inserts its page without a
  content hash, which under the new semantics models a changed file.
  Its fixture gains the file's matching hash — assertions unchanged —
  so it keeps verifying the unchanged-file skip it was written for.
  `TestReIngestLearnings_ExistingPageNotOverwritten` pins the
  insert-only guarantee "existing store pages are not demoted" from the
  dewey-distill delta spec (council round-3 G10); this change supersedes
  that scenario (see proposal, Modified Capabilities): the file is the
  authority, so its fixture page (empty stored hash, hand-edited
  metadata, tier `validated`) converges to the file — count 1, tier
  `draft`, file-derived properties, blocks re-persisted. The state the
  old scenario protected (a legitimately promoted learning page) is
  unreachable — learning pages are never promotable — and the
  requirement's core (file tier claims never honored, always `draft`)
  is preserved and enforced on both paths. Every other existing
  re-ingestion test passes unmodified, and startup behavior for vaults
  that don't hit these cases is byte-identical.
- **Partial-failure window on the changed path (accepted risk, recorded
  at code review iteration 3).** The changed-file sequence
  `DeleteBlocksByPage` → `UpdatePage` (new hash) → `PersistBlocks` →
  `GenerateEmbeddings` is not transactional. A `PersistBlocks` failure
  after `UpdatePage` leaves the row holding the new hash with its blocks
  deleted — the next startup's hash match takes the cheap path and the
  page stays block-less until the file changes again or `dewey reindex`
  runs. A `DeleteBlocksByPage` failure is logged and the sequence
  proceeds, so `PersistBlocks` appends the new blocks beside the
  un-deleted old ones with the hash already converged. Both windows need
  a transient store failure mid-sequence, and the insert path has had
  the same exposure all along; the changed-file path widens it. Accepted
  rather than fixed here: the real fix is a store-level transactional
  boundary (also raised at spec review) and a one-word hardening —
  `continue` on a delete failure, leaving the stale hash to retry the
  whole convergence at the next startup — both follow-up work.
- **No schema change.** Everything lands in existing columns
  (`content_hash`, `properties`, `tier`, `category`); no migration and
  no `schema_version` bump, honoring the epic's constraint.
