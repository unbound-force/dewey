## Context

`tokenize` splits on non-word characters, where word characters
include `-` and `_`. `Search` intersects the posting lists of every
query term (AND), capped at `limit`. A compound term has one posting
list under its whole spelling only.

## Goals / Non-Goals

### Goals
- Truncated and space-split forms of hyphenated IDs find them.
- Exact full-ID matches keep first place.

### Non-Goals
- General prefix or fuzzy matching (`adj` does not match `adjud`).
- Relevance ranking beyond exact-first.
- Changing the store-backed `LIKE` query or its dedupe semantics (the
  merge gate now keys on exact matches — see D4).

## Decisions

### D1 Index components alongside the whole term

`tokenize` is unchanged (it also feeds `SortByRelevance`). Indexing
adds the components of each compound term (`splitCompound`: split on
`-` / `_`, keep parts of length ≥ 2 bytes, matching `tokenize`'s
term-length filter). Removal already walks the page's recorded term
set, so components are removed with the page.

### D2 Two-pass query, exact first

Pass 1 is today's exact-term AND query. If the query contains a
compound term and pass 1 returned fewer than `limit` results, pass 2
runs the AND query over the query's terms with each compound replaced
by its components, appending unseen blocks up to `limit`. A query
with no compound terms is a single pass, identical to today.

### D3 Components-only compounds

A compound whose parts are all shorter than 2 bytes (`a-b`)
yields no components and stays a whole-term lookup. A mixed compound
(`x-kb`) yields only its qualifying parts (`[kb]`); the dropped
1-byte part also loses its AND constraint in pass 2, so `x-kb`
matches every block containing `kb` — broader than a whole-term lookup,
the same set the surviving component matches on its own.

### D4 Store merge keys on exact matches

`FullTextSearch` gates the store merge on the pass-one exact-match
count rather than the total hit count, and interleaves store-only
exact-substring hits ahead of pass-two component fills: a store-only
page holding the full compound ID cannot be displaced by
component-only matches. The `LIKE` query itself is unchanged.

## Risks / Trade-offs

- **Broader pass 2.** `foo-bar` can now match a block containing
  `foo` and `bar` separately, after all exact matches. This is the
  standard analyzer behavior of common search engines; exact matches
  still come first.
- **Index growth** is bounded by the component count of compound
  words in indexed content.
