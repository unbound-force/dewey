## Context

`SemanticSearch` and `SemanticSearchFiltered` query the store by
vector similarity, then `toSemanticResults` enriches each result with
page tier, category, and created_at via a per-page cache. The tier
filter is a post-filter with 3× over-fetch.

## Goals / Non-Goals

### Goals
- Expose `validity` / `as_of` on semantic results.
- Filter by validity; rank stale and superseded content lower.

### Non-Goals
- Keyword `search` enrichment.
- Validity-weighted ranking, validity filtering, or over-fetch for
  `similar` — its results still carry `validity` / `as_of` through the
  shared `toSemanticResults` converter (enrichment-only).
- Per-claim validity inside a page body.
- Tier-weighted ranking.

## Decisions

### D1 Enrichment from page properties

`toSemanticResults` already loads each page; it also decodes
`properties` and copies string `validity` and `as_of`. All three
semantic tools — `semantic_search`, `semantic_search_filtered`, and
`similar` — route through this converter, so all three carry the
fields.

### D2 Weights

`superseded` 0.6, `stale-unverified` 0.85, all others (including
absent, `current`, `historical`) 1.0. Historical knowledge is not
penalized: it is immune to code evolution by definition. Ordering is a
stable sort by `similarity × weight`; `similarity` in the response
stays raw.

### D3 Over-fetch

When ranking or a post-filter (tier or validity) is active, fetch
`max(3 × limit, 30)` (the existing tier rule), then filter, rank, and
trim to `limit`. With `ignore_validity` and no post-filter, fetch
`limit`, as today.

### D4 Validity filter

Exact match on the page's `validity`. Pages without validity are
excluded when the filter is set (the tier filter's rule for unknown
pages).

## Risks / Trade-offs

- **Ordering change** for vaults that already use `validity`. Pages
  without it are unaffected; `ignore_validity` restores raw order.
- **Weights are fixed.** Configurable weights can follow if needed.
