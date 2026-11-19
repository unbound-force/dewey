# Delta Spec: learning-reingest

<!--
  Requirement numbering allocation
  ================================
  Capability: learning-reingest (prefix LR)
  Allocated to pages-for-deleted-learning-and-knowledge:
    LR-FR-003 .. LR-FR-003
  Reserved for future changes: LR-FR-004 .. LR-FR-099
  Custody: extends LR-FR-001/LR-FR-002 from
    learning-reingest-on-file-change (startup re-ingest);
    re-uses spec 015 FR-003 family.
-->

## ADDED Requirements

### Requirement: Deleted learning files prune their pages at
### startup

**ID**: LR-FR-003

During `dewey serve` startup, after `reIngestLearnings` has
scanned `.uf/dewey/learnings/` and inserted or refreshed pages
for files that exist, Dewey MUST delete every `learning/*` page
whose file no longer exists. The page set MUST be the pages
whose `source_id` is `learning`; the file set MUST be the
`.md` entries just scanned, filtered as the scan filters them
(skip directories, skip non-`.md`, skip files whose
`ValidLearningIdentity` fails). A learnings directory that
does not exist MUST be treated as an empty file set; any other
directory scan error (permissions, I/O, transient failure) or
a page-listing error MUST skip pruning for that pass entirely,
log a warning, and leave every `learning/*` page in place —
pruning fails closed, so a transient fault MUST NOT delete
pages. Deletion MUST remove the page row and MUST cascade to
its blocks and embeddings through the existing foreign keys. A
per-page deletion failure MUST be logged as a warning, pruning
MUST continue with the remaining orphans, and the logged count
MUST include successful deletions only; a failed page is
retried on the next startup. When the pruned count is greater
than zero, Dewey MUST log it with the same verbosity as the
re-ingest count.

#### Scenario: A pulled deletion disappears after restart

- **GIVEN** a vault where `.uf/dewey/learnings/` holds
  `auth-20260101T000000-alice.md` and
  `billing-20260102T000000-bob.md` and both are indexed
- **AND** a `git pull` deletes `billing-*.md` from the directory
- **WHEN** `dewey serve` starts and `reIngestLearnings` runs
- **THEN** page `learning/billing-20260102T000000-bob` does not
  exist in the store
- **AND** page `learning/auth-20260101T000000-alice` still exists
  and is unchanged
- **AND** no blocks or embeddings remain for the deleted page

#### Scenario: Nothing is deleted when nothing was deleted

- **GIVEN** a vault where every `learning/*` page has a matching
  file on disk
- **WHEN** startup re-ingestion runs
- **THEN** no `learning/*` page is deleted
- **AND** no pruning log line is emitted

#### Scenario: A learning deleted manually without a commit

- **GIVEN** a vault where the operator removes a learning `.md`
  file by hand
- **WHEN** the server restarts
- **THEN** the corresponding `learning/*` page is removed from
  the index
- **AND** `semantic_search` no longer returns it

#### Scenario: A deleted and re-added file restores cleanly

- **GIVEN** a learning page was previously pruned because its
  file was deleted
- **AND** a later `git pull` re-adds a learning file with the
  same identity but different content
- **WHEN** the server starts
- **THEN** the page is inserted from the new file with the new
  content hash and blocks

#### Scenario: A transient scan failure prunes nothing

- **GIVEN** a vault whose `.uf/dewey/learnings/` directory
  exists but cannot be read (permissions or I/O error)
- **WHEN** startup re-ingestion runs
- **THEN** no `learning/*` page is deleted
- **AND** the scan failure is surfaced (warning or error log)

#### Scenario: A deletion failure mid-prune keeps the failed
#### page

- **GIVEN** a vault with two orphaned `learning/*` pages
- **AND** the first page deletion fails with a store error
- **WHEN** startup pruning runs
- **THEN** the failed page remains in the store
- **AND** the second orphan is still deleted
- **AND** the deletion failure is logged as a warning
- **AND** the pruned count logged for the pass is one

## MODIFIED Requirements

None.

## REMOVED Requirements

None.

<!-- scaffolded by uf vdev -->
