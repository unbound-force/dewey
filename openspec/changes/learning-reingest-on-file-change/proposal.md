# Proposal: Re-ingest Learnings When Their Files Change

## Why

`reIngestLearnings` (main.go, package main) re-reads a learning file at
startup only when `learning/<identity>` is missing from the store. When
the page exists, the file is skipped, whatever changed in it. Verified
against `main` (`50e8ff8`): the existing-page branch (main.go:1109)
skips unconditionally.

So a learning edited in place — a `git pull` that fast-forwards the
repo, a stealth overlay syncing several checkouts — keeps its old body
and properties in the index until someone runs `dewey reindex`. The
file on disk is right and the index quietly isn't. This matters
wherever several checkouts share learnings through git; gh-hud flags
`learnings changed` and demands a reindex today (jflowers/gh-hud#138)
and could drop that nag once this ships.

Issue #220 already solved the same problem for knowledge stores:
`tools.IndexKnowledgeStore` compares the page's `ContentHash`,
rebuilds blocks and embeddings only when the content changed, and
updates tier and properties in place when only those differ. This
change makes `reIngestLearnings` do the same for learnings: unchanged
learnings stay cheap at startup, and a pull followed by a server
restart is enough to converge the index.

## What Changes

1. **Content-hash comparison in `reIngestLearnings` (main.go).** The
   existing-page check becomes the three-branch shape
   `IndexKnowledgeStore` established: page missing → insert (today's
   behavior, unchanged); content hash matches → no block or embedding
   work, but the page row's file-derived fields (properties, category)
   and tier are refreshed in place when they differ; hash mismatch →
   delete the old blocks, update the page row from the file, re-persist
   blocks, and regenerate embeddings. The hash input stays the file
   body — exactly what the insert path computes today — so existing
   rows keep matching.
2. **`store_learning` hash alignment (tools/learning.go).**
   `store_learning` hashes the raw `information` text while the file
   body it writes is that text plus a trailing newline, and re-ingestion
   hashes the trimmed body. For unpadded text the two agree; for padded
   text they diverge. `store_learning` now hashes the trimmed text, so
   newly stored rows agree with their files from the start. Existing
   divergent rows converge once (a single re-ingest), then match on
   every later startup.
3. **Count semantics.** `reIngestLearnings`'s return value counts full
   re-ingests (missing pages and content changes) only; properties-only
   refreshes do not count. The startup log line stays silent on cheap
   starts, as it is today.

## Capabilities

### New Capabilities
- `learning-reingest`: learnings are re-ingested from their files
  whenever the file content differs from the indexed page — not only
  when the page is missing — with unchanged files costing no block or
  embedding work.

### Modified Capabilities
- `knowledge-distillation` — Learning Re-ingestion Tier Neutrality (from
  the dewey-distill change's delta spec, not yet synced to main specs):
  the requirement's rationale clause "Re-ingestion only fills pages
  absent from the store" and its "Existing store pages are not demoted"
  scenario are superseded by this change's file-as-authority convergence
  (LR-FR-001/LR-FR-002). The requirement's core — file tier claims are
  never honored and re-ingested learnings always land at `draft`, with a
  warning for forged claims — is preserved and extended to the update
  path: a store row whose tier drifted to a non-draft value converges
  back to `draft` on the next startup (learnings clamp to draft on every
  route; learning pages are never promotable, so the demotion the old
  scenario guarded against has no legitimate state to protect). The
  requirement's two missing-page scenarios are unchanged. Speckit spec
  015 (FR-003) is extended without altering it.

### Removed Capabilities
- None.

## Impact

- **Files modified**: `main.go` (`reIngestLearnings` three-branch
  comparison), `tools/learning.go` (hash alignment, one expression),
  `main_test.go`, `tools/learning_test.go`.
- **Runtime behavior**: `dewey serve` startup already reads and parses
  every learning file and calls `GetPage` per file; the only added work
  for unchanged files is one SHA-256 over the body plus an occasional
  `UpdatePage` when properties drifted. Changed or missing files are
  fully indexed and embedded, as a fresh store would do. No CLI flags,
  MCP tools, config, file format, or frontmatter changes.
- **Schema**: none. The comparison uses the existing `content_hash`,
  `properties`, `tier`, and `category` columns; no migration, no
  `schema_version` bump.
- **One-time convergence**: rows whose stored hash was computed by
  `store_learning` over untrimmed text (or pre-hash rows with an empty
  `content_hash`) re-ingest once on the first startup after upgrade,
  then match on every later start.
- **Test updates**: two existing tests pin the pre-change skip behavior
  and update with this change. `TestReIngestLearnings_SkipsExisting`
  inserts its page without a content hash, so under the new semantics it
  would model a changed file; its fixture gains the file's matching
  hash — assertions unchanged — so it keeps verifying the unchanged-file
  skip it was written for. `TestReIngestLearnings_ExistingPageNotOverwritten`
  pins the insert-only guarantee "existing store pages are not demoted"
  from the dewey-distill delta spec (council round-3 G10), which this
  change supersedes (see Modified Capabilities): its fixture page has an
  empty stored hash and hand-edited metadata, so under LR-FR-001 it
  models a changed file and converges to it. The test updates to pin
  that convergence. Every other existing re-ingestion test passes
  unmodified.
- **Consumers**: gh-hud can stop flagging `learnings changed` as a
  reindex demand (jflowers/gh-hud#138); a server restart suffices.

## Constitution Alignment

Assessed against the Unbound Force org constitution.

### I. Autonomous Collaboration

**Assessment**: PASS

Learning files remain the self-sufficient durable record: any checkout
— fresh clone, overlay, or fast-forwarded pull — converges to the same
index state from the files alone, with no coordination beyond a server
restart.

### II. Composability First

**Assessment**: PASS

No new dependencies, configuration, or CLI surface. Vaults without a
learnings directory behave exactly as before (no-op), and the 37
inherited graphthulhu tools are untouched.

### III. Observable Quality

**Assessment**: PASS

The existing startup log ("learnings re-ingested from files") reports
real work with its count; unchanged files produce no misleading
output. The convergence decision (hash match vs. mismatch) is
deterministic and auditable from the stored `content_hash` and the
file on disk.

### IV. Testability

**Assessment**: PASS

`reIngestLearnings` already takes its store, embedder, and vault path
as parameters; the three branches are exercised with an in-memory
store, `t.TempDir()` files, and a counting embedder fake that proves
unchanged files trigger no embedding calls.
