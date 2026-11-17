# Proposal: Re-index Knowledge Stores From Their Files

## Why

Curated knowledge stores (`.uf/dewey/knowledge/<store>/`, spec 015) are
file-backed, but nothing rebuilds their pages from those files. Verified
against v3.7.0 and `main` (`97acb58`):

- `tools.IndexKnowledgeStore` has exactly two callers: the `curate` MCP
  tool (`Curate.autoIndexKnowledgeStore`) and background curation
  (`backgroundCurateStore`, main.go), and the latter only when a
  curation pass **created files** (`filesCreated > 0`).
- The vault scan and the disk source both skip hidden directories, so
  `.uf/dewey/knowledge/**` is never reached by `indexVault` or
  `disk-local`.
- Server startup's deferred index re-ingests learnings from files
  (`reIngestLearnings`, FR-003) but has no knowledge-store equivalent.

So after `dewey reindex`, graph.db deletion, or a fresh clone, every
knowledge-store page is missing from the store until a curation run
happens to write a new file. A store whose files are committed to the
repository (hand-curated, generated, or with no sources configured)
never comes back. Measured on a real overlay vault: 461
`knowledge/cross-repo-contracts/*` pages and 347 pages of a source-less
store were absent after `dewey reindex` and server restart.

Separately, `IndexKnowledgeStore` writes pages without `Properties`.
The vault and source indexers persist frontmatter as the page's
properties JSON (`vault/vault_store.go`, `vault/index_documents.go`),
but the knowledge-store path drops it. Property readers therefore
never see a curated page's frontmatter — in particular
validity-aware retrieval (`pageValidity`, `tools/semantic_validity.go`)
reads `validity` / `as_of` from `Properties`, so curated pages are
never down-weighted when `stale-unverified` / `superseded` and are
excluded by `semantic_search_filtered` with a `validity` filter.

Related: #210 (workspace-layout trust tiers) names the missing
`.uf/dewey/knowledge/**` ingestion route as one of its root causes;
this change adds that route for the store pages themselves without
touching trust pathspecs.

## What Changes

1. **Startup re-index (main.go).** A new `reIndexKnowledgeStores`
   function loads `knowledge-stores.yaml` and calls
   `tools.IndexKnowledgeStore` for every configured store. The deferred
   index calls it right after `reIngestLearnings`, under the same
   `indexMu` hold. Missing config is a no-op; an invalid config is
   logged as a warning and does not stop indexing.
2. **Properties persistence (tools/curate.go).** `IndexKnowledgeStore`
   marshals the parsed frontmatter with `json.Marshal` (the encoding
   the other indexers use) and stores it as the page's `Properties` on
   insert and on content-changed update. On the unchanged-content path
   it already refreshes the cached tier; it now also refreshes
   `Properties` when they differ, so rows written by earlier binaries
   are backfilled without re-persisting blocks or re-embedding.

## Capabilities

### New Capabilities
- None.

### Modified Capabilities
- `curated-knowledge-stores`: knowledge-store pages are rebuilt from
  their files at server startup (CKS-FR-001) and carry their
  frontmatter as page properties (CKS-FR-002).

### Removed Capabilities
- None.

## Impact

- **Files modified**: `main.go` (deferred index call +
  `reIndexKnowledgeStores`), `tools/curate.go` (`IndexKnowledgeStore`
  properties), `main_test.go`, `tools/curate_test.go`.
- **Runtime behavior**: `dewey serve` startup indexes configured
  knowledge stores. Unchanged pages cost a file read, a parse and a
  `GetPage` (content-hash match skips blocks and embeddings); changed
  or missing pages are fully indexed and embedded, as curation already
  does. No CLI flags, MCP tools, schema, or config changes.
- **Search**: curated pages with `validity` frontmatter now take part
  in validity ranking and filtering — by design.
- **Backward compatibility**: fully compatible. Existing rows are
  upgraded in place on the next startup.

## Constitution Alignment

### I. Autonomous Collaboration

**Assessment**: PASS

Knowledge-store files become the self-sufficient durable record —
any agent or fresh clone recovers the same pages from them, as with
learnings.

### II. Composability First

**Assessment**: PASS

No new dependencies or configuration. Vaults without
`knowledge-stores.yaml` behave exactly as before.

### III. Observable Quality

**Assessment**: PASS

Startup logs the re-indexed count ("knowledge stores re-indexed from
files"), and curated pages expose their validity and as_of to the
retrieval layer that reports them.

### IV. Testability

**Assessment**: PASS

`reIndexKnowledgeStores` takes its store, embedder and vault path as
parameters (the `reIngestLearnings` pattern) and is unit-tested with
an in-memory store; the properties behavior is tested directly on
`IndexKnowledgeStore`.
