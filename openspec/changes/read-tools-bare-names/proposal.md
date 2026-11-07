<!-- originating_issue: 140 -->

## Why

`list_pages` shows compiled articles under their bare topic, and
`promote` accepts bare names, but read tools only resolve the
canonical form. The natural flow
`store_compiled(tag: "gateway")` → `list_pages` → `get_page("gateway")`
returns "page not found". The PR #138 review council found this gap.

## What Changes

On a miss, the store fallback behind `get_page` (and every internal
name-based read it shares) tries, in order: the exact name (case-folded,
as today), `compiled/<name>`, then `learning/<name>`. The first
existing page wins. Names containing `/` skip the shorthand.

Vault files keep precedence, and with no store configured nothing
changes. `get_block` takes a UUID, so it has no name to resolve. Page
writes are unchanged: the write guard keeps its exact lookup, so
`create_page("gateway")` still creates a vault page, which then takes
precedence on reads.

## Capabilities

### New Capabilities
- None.

### Modified Capabilities
- `get_page`: resolve bare names of compiled articles and learnings from
  the store. Every name-based read through the vault client — `get_links`
  forward direction, `traverse` start pages, `get_whiteboard`,
  `bulk_update_properties` — shares the resolution.

### Removed Capabilities
- None.

## Impact

- `vault/vault_store.go` — `findStorePage`, used by `GetStorePage`.
- `vault/vault_store_test.go`, `vault/vault_test.go`.
- `server.go`, `types/tools.go` descriptions; README.
- No store query changes; a miss costs up to two more indexed lookups.

## Constitution Alignment

Assessed against the Dewey project constitution (v1.4.0).

### I. Composability First

**Assessment**: PASS

No new dependencies. With no store the client behaves as before.

### II. Autonomous Collaboration

**Assessment**: PASS

Agents can read back a page by the name `list_pages` showed them.

### III. Observable Quality

**Assessment**: PASS

The returned entity carries the canonical page name.

### IV. Testability

**Assessment**: PASS

Resolution order and precedence are unit tested against an in-memory
store.
