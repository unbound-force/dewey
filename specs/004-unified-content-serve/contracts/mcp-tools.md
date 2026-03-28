# MCP Tool Contract Changes: Unified Content Serve

**Date**: 2026-03-28

## Overview

No MCP tool input schemas change. No new MCP tools are added. The change is in **result behavior**: existing tools now return results that include external-source pages alongside local vault pages.

## Tool Categories and Impact

### No Contract Changes (input/output schema identical)

All 40 MCP tools (37 inherited from graphthulhu + 3 semantic search tools added in spec 001) retain their existing input schemas and output JSON structures. The change is additive — results may now include pages from external sources.

### Behavioral Changes (additive results)

#### Search Tools
- `dewey_search`: Results may include external-source pages. Each result's page name includes the source namespace prefix (e.g., `github-myorg/issues/42`).
- `dewey_full_text_search`: Same — external content blocks appear in search results.
- `dewey_semantic_search`: Cosine similarity results include external-source blocks (if embeddings were generated during indexing).
- `dewey_similar`: Similar pages may include external-source pages.
- `dewey_semantic_search_filtered`: Source-based filtering via existing `source_id` filter parameter.

#### Navigation Tools
- `dewey_get_all_pages`: Returns all pages including external sources. Page names are namespaced.
- `dewey_get_page`: Can retrieve external-source pages by their namespaced name.
- `dewey_get_page_blocks_tree`: Returns block tree for external-source pages (reconstructed from store).
- `dewey_get_block`: Can retrieve blocks from external-source pages by UUID.
- `dewey_get_page_linked_references`: Backlinks now include cross-source references.

#### Analysis Tools
- `dewey_graph_analysis`: External pages participate in graph metrics (degree, centrality).
- `dewey_dead_links`: Links from external pages to non-existent pages appear as dead links.

#### Status Tools
- `dewey_health`: Reports external page count alongside local page count.

### Write Tool Behavior (error on external pages)

The following tools return an error when targeting an external-source page:

- `dewey_create_page` — N/A (creates new pages, not affected)
- `dewey_append_block_in_page` — Error if target page is external
- `dewey_prepend_block_in_page` — Error if target page is external
- `dewey_insert_block` — Error if target block belongs to external page
- `dewey_update_block` — Error if target block belongs to external page
- `dewey_remove_block` — Error if target block belongs to external page
- `dewey_move_block` — Error if source or target block belongs to external page
- `dewey_delete_page` — Error if target page is external
- `dewey_rename_page` — Error if target page is external

**Error format** (structured JSON, same as existing MCP error responses):
```json
{
  "error": "page \"github-myorg/issues/42\" is read-only (source: github-myorg)"
}
```

### CLI Command Changes

- `dewey status`: Output now includes per-source page counts.
- `dewey index`: Now persists blocks, links, and embeddings (not just page metadata). Purges orphaned sources.
- `dewey search`: Results include external-source content (same as MCP search tools).

## Backward Compatibility

- All 37 inherited graphthulhu MCP tools produce identical results for local vault content.
- No input schema changes — existing MCP clients continue to work without modification.
- External pages are additive — they appear in results alongside local pages but never replace them.
- Page name namespacing ensures no collisions with existing local pages.
