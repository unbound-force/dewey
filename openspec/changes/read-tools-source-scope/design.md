# Design: Scope Every Read Tool by Source

## Context

See proposal. Constitution alignment: all four principles PASS (see
proposal) — additive optional arguments, no new dependencies, honest
errors, isolated tests.

## Goals / Non-Goals

### Goals

- A caller can scope any of the four everyday read tools to one source
  (and, where it makes sense, one tag) with one argument.
- A scoped semantic call returns exactly what `semantic_search_filtered`
  returns for the same filters.
- Unscoped calls are unchanged.

### Non-Goals

- `source_type`, `has_property`, `tier` and `validity` on the plain
  tools. `semantic_search_filtered` remains the tool for those; adding
  them everywhere would duplicate it.
- Scoping `get_page`, `traverse` or other name-addressed tools: a known
  page name is already a deliberate choice.
- A store-side index on `pages.source_id` for the full-text path.

## Decisions

### D1 Reuse `SearchSimilarFiltered` for the semantic tools

`semantic_search` and `similar` call a small helper,
`searchSimilarScoped`, that runs `SearchSimilar` when the filters are
empty and `SearchSimilarFiltered` otherwise. One SQL filter means one
definition of "in scope"; a parity test pins it.

### D2 Resolve full-text scope per page through the store

`search` and `find_by_tag` read through the backend (vault in-memory
index merged with store pages), whose hits carry page names but not
sources. The handler resolves each distinct page once with
`GetPageFoldCase` (the vault keys pages case-insensitively) and applies
`source_id` / `has_tag` in Go with the same rules as the SQL filter:
exact `source_id`; `has_tag` when lower-cased properties contain the
tag or lower-cased block content contains `#tag`. A page absent from
the store never matches an active scope, as the SQL join drops it.

Alternative rejected: query store blocks directly when scoped. That
would change `search` semantics (token index vs. substring `LIKE`) only
for scoped calls, and lose the brute-force/backends parity.

### D3 Over-fetch, then filter, for indexed full-text search

The backend's `FullTextSearch` takes a limit. A scoped call asks for
`max(10×limit, 200)` hits, keeps in-scope ones, and trims to the limit
(`scopedFetchLimit`). Semantic tools already over-fetch for tier and
validity (`semanticFetchLimit`); full-text uses a larger factor because
one topic is often a small share of the corpus. The brute-force path
needs no over-fetch: it skips out-of-scope matches before they count.

### D4 `WithStore` setter, error without a store

`NewSearch(b)` keeps its signature (many tests and call sites);
`WithStore` mirrors `Learning.WithAsOfResolver`. A scoped call without
a store returns `source_id/has_tag filters unavailable: no persistent
store configured.` — honoring the request is impossible, and ignoring
it would leak other sources into a caller that asked for one.

### D5 `find_by_tag` takes `source_id` only

It already selects by tag; `has_tag` would be a second, differently
matched tag filter on the same call.

## Risks / Trade-offs

- A scope matching fewer than `limit` of the first `max(10×limit,200)`
  index hits returns fewer results than exist. Acceptable for a scoped
  keyword search; `semantic_search_filtered` filters in SQL and has no
  such cap.
- One `GetPageFoldCase` per distinct hit page per scoped call. Bounded
  by the fetch limit.
