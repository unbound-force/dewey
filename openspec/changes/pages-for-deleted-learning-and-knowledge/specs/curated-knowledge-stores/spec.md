# Delta Spec: curated-knowledge-stores

<!--
  Requirement numbering allocation
  ================================
  Capability: curated-knowledge-stores (prefix CKS)
  Allocated to pages-for-deleted-learning-and-knowledge:
    CKS-FR-003 .. CKS-FR-003
  Reserved for future changes: CKS-FR-004 .. CKS-FR-099
  Custody: extends CKS-FR-001/CKS-FR-002 from
    knowledge-store-reindex (startup re-index and properties);
    re-uses speckit spec 015 store family.
-->

## ADDED Requirements

### Requirement: Deleted store files prune their pages at startup

**ID**: CKS-FR-003

During `dewey serve` startup, after each
`IndexKnowledgeStore` invocation has scanned its store
directory and inserted or refreshed pages for files that exist,
Dewey MUST delete every page of that store's source
(`knowledge-{store}`) whose file no longer exists in the store
directory. The file set MUST be the `.md` entries just scanned,
filtered as the scan filters them (skip non-`.md`, skip
`_index.md`, skip dot-files). A store directory that does not
exist MUST be treated as an empty file set for that store's
pages; any other directory scan error (permissions, I/O,
transient failure) or a page-listing error MUST skip pruning
for that store entirely, log a warning, and leave that store's
pages in place — pruning fails closed, so a transient fault
MUST NOT delete pages. This requirement covers stores still
configured in `knowledge-stores.yaml`; pages of a store whose
config entry was removed are out of scope for this change.
Deletion MUST remove the page row and MUST cascade to its
blocks and embeddings through the existing foreign keys. A
per-page deletion failure MUST be logged as a warning, pruning
MUST continue with that store's remaining orphans, and the
logged count MUST include successful deletions only; a failed
page is retried on the next startup. When the pruned count for
a store is greater than zero, Dewey MUST log it with the same
verbosity as the per-store indexed count; the wrapper
`reIndexKnowledgeStores` MAY aggregate the per-store counts into
one line.

#### Scenario: A store file removed by a pull disappears after
#### restart

- **GIVEN** a vault whose `knowledge-stores.yaml` lists store
  `team-kb` with source `knowledge-team-kb`
- **AND** store directory `.uf/dewey/knowledge/team-kb/` holds
  `decision-a.md`, `decision-b.md`, `_index.md`
- **AND** all three non-index pages are indexed
- **AND** a `git pull` removes `decision-b.md`
- **WHEN** `dewey serve` starts and `reIndexKnowledgeStores`
  runs
- **THEN** page `knowledge/team-kb/decision-b` does not exist in
  the store
- **AND** page `knowledge/team-kb/decision-a` still exists and
  is unchanged
- **AND** no blocks or embeddings remain for the deleted page
- **AND** no page ever existed for `_index.md`

#### Scenario: Nothing is deleted when nothing was deleted

- **GIVEN** every `knowledge/<store>/*` page has a matching file
  in its store directory
- **WHEN** startup re-indexing runs
- **THEN** no `knowledge/<store>/*` page is deleted for that
  store
- **AND** no pruning log line is emitted for it

#### Scenario: A store directory removed entirely

- **GIVEN** a store `obsolete` still configured in
  `knowledge-stores.yaml` whose pages are indexed
- **AND** the operator deletes `.uf/dewey/knowledge/obsolete/`
  from disk
- **WHEN** the server restarts
- **THEN** every page whose `source_id` is `knowledge-obsolete`
  is removed from the store
- **AND** no blocks or embeddings remain for those pages

#### Scenario: Search stops returning deleted curated pages

- **GIVEN** a curated page that was indexed from a store file
- **AND** that file is then deleted and the server restarted
- **WHEN** `semantic_search` or `semantic_search_filtered` is
  called with a query matching the deleted page
- **THEN** the response does not contain the deleted page

#### Scenario: A transient scan failure prunes nothing

- **GIVEN** a configured store whose directory exists but
  cannot be read (permissions or I/O error)
- **WHEN** startup re-indexing runs
- **THEN** no page of that store's source is deleted
- **AND** the scan failure is surfaced (warning or error log)

#### Scenario: A deletion failure mid-prune keeps the failed
#### page

- **GIVEN** a configured store with two orphaned pages
- **AND** the first page deletion fails with a store error
- **WHEN** startup pruning runs
- **THEN** the failed page remains in the store
- **AND** the second orphan is still deleted
- **AND** the deletion failure is logged as a warning
- **AND** the store's pruned count logged for the pass is one

## MODIFIED Requirements

None.

## REMOVED Requirements

None.

<!-- scaffolded by uf vdev -->
