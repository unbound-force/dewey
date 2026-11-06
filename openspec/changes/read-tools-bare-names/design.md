## Context

`Client.GetPage` and `GetPageBlocksTree` check the in-memory index,
then call `VaultStore.GetStorePage`, which folds case in SQL.
`resolvePromotePage` (`tools/promote.go`, extended by #145) has four
tiers and rejects ambiguity. `StorePageMeta` backs the write guard
with its own exact lookup.

## Goals / Non-Goals

### Goals
- `get_page("gateway")` finds `compiled/gateway` at every session age.
- Keep vault-file precedence and no-store parity.

### Non-Goals
- Knowledge-store suffix or source-doc resolution.
- Bare-name write resolution.
- Resolving backlinks under the canonical name.

## Decisions

### D1 Resolve in the store fallback

The shorthand goes in `VaultStore.GetStorePage`, so both read methods
get it and in-memory hits still win. Startup-hydrated pages are keyed
by canonical name, so a bare name misses in memory and reaches the
fallback at every session age.

### D2 Two prefixes, not promote's resolver

Issue #140 asks for exact, then `compiled/`, then `learning/`, first
existing wins. `resolvePromotePage` is deliberately stricter: promote
mutates a trust tier, so an ambiguous name must fail. A read is
harmless and has an obvious preference: `list_pages` shows compiled
articles under the bare topic. Its knowledge and source-doc tiers
would also add a page scan to every read miss, and `tools` cannot be
imported from `vault`.

### D3 Write guard unchanged

`StorePageMeta` keeps its exact lookup, so a vault page may still be
created under a compiled article's bare topic. `bulk_update_properties`
reads through `GetPageBlocksTree`, but its `UpdateBlock` write is
refused for a store block, so the page reports as failed, as it did
before.

## Risks / Trade-offs

- **Promote and reads differ on a name matching both namespaces.**
  Promote reports ambiguity; a read serves the compiled article.
- **`get_page` backlinks** still use the requested name, so a bare
  request lists links to the bare name only.
