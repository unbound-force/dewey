<!-- originating_issue: 149 -->

## Why

Keyword `search` finds hyphenated identifiers only in their full
form. The in-memory index (`vault/search_index.go`) treats `-` and
`_` as word characters, so `KB-REVIEW-ADJUD-01` is indexed as the
single term `kb-review-adjud-01`. Lookup is exact-term, so:

- `KB-REVIEW-ADJUD-01` → matches
- `KB-REVIEW-ADJUD` (truncated) → no results
- `KB REVIEW ADJUD 01` (space-split) → no results

Agents cite knowledge by ID and routinely truncate or reformat it, so
every consumer has needed a "use the FULL hyphenated ID" workaround.

Part of #153 (umbrella).

## What Changes

- Indexing: a term containing `-` or `_` is indexed whole **and** as
  its component parts (each at least 2 characters).
- Querying: exact-term matches come first, exactly as today. When the
  query has compound terms and fewer than `limit` exact hits, the
  remainder is filled with blocks matching every component (AND),
  without duplicates.

Result: the three query forms above all find the block; the full ID
still ranks its exact matches first.

## Capabilities

### New Capabilities
- None.

### Modified Capabilities
- `search` (in-memory full-text index): compound terms are searchable
  by component and by prefix-of-components.

### Removed Capabilities
- None.

## Impact

- `vault/search_index.go` — component indexing; two-pass query.
- `vault/search_index_test.go` — the three issue forms, ordering,
  dedupe, limit.
- `vault/vault.go` — store merge in `FullTextSearch` now gated on
  the exact-match (pass-1) count instead of total hits, so
  store-only exact-ID pages surface before component-only matches
  consume the limit; the `LIKE` query itself is unchanged.
- README search row.
- Website docs-sync issue: unbound-force/website#390 (user-facing
  search behavior change; `search` MCP tool and `dewey search` CLI
  now match compound IDs by component).
- No change to `semantic_search` or
  `find_by_tag`.

Index size grows by the component terms of compound words only.

## Constitution Alignment

Assessed against the Dewey project constitution (v1.4.0).

### I. Composability First

**Assessment**: PASS

No new dependencies; in-memory index only.

### II. Autonomous Collaboration

**Assessment**: PASS

Agents can retrieve knowledge by the ID forms they actually produce.

### III. Observable Quality

**Assessment**: PASS

Result shape unchanged; exact matches keep priority.

### IV. Testability

**Assessment**: PASS

Pure index functions covered by unit tests.
