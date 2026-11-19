# Design: `dewey reindex` Restores Learnings and Knowledge Stores

## Context

See proposal. Constitution alignment: all four principles PASS.

## Goals / Non-Goals

### Goals

- After `dewey reindex` returns, `graph.db` holds every learning and
  knowledge-store page, as it would after a `dewey serve` start.

### Non-Goals

- Watching `.uf/dewey/learnings/` while serving (see proposal).
- Changing `dewey index` or the MCP `reindex` tool. The MCP tool keeps
  the protected `learning` source; knowledge stores under it are out of
  scope here.

## Decisions

### D1 One shared function for both rebuild paths

`restoreFileBackedPages` holds the two passes and their logging, so
serve startup and reindex cannot drift. It takes the store, embedder,
vault path and chunk size as parameters, like the functions it calls.

### D2 Run after sources, before the stamp

Order matches serve: sources first, then file-backed pages. Running
before `recordIndexBuild` means the stamp marks a complete index.
Failures stay warnings, as on serve: a learning that fails to parse
should not fail an otherwise good rebuild.

## Risks / Trade-offs

- With embeddings enabled, reindex now embeds learnings too, so it
  takes longer by the time serve startup would have spent anyway.
