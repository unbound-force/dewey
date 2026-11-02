## Why

MCP read tools cannot see pages that live in the internal SQLite store
(`.uf/dewey/graph.db`). Pages created via `store_learning` and
`store_compiled` are persisted to the store but are invisible to
`get_page`, `list_pages`, `search`, and `get_block` until the server is
restarted — and `dewey promote` cannot reliably transition them
(#90, #91).

The two data layers are only half-bridged today:

- **Vault layer** — `vault.Client` builds an in-memory page index from
  vault `.md` files. All MCP read tools (`get_page`, `list_pages`,
  `search`, `get_block`, analyze tools) operate on this index through
  `backend.Backend`.
- **Store layer** — `store.Store` (SQLite) is the durable index over
  everything: vault files, external sources (GitHub, web, code,
  knowledge stores), learnings (`learning/{identity}`), and compiled
  articles (`compiled/{tag}`). `store_learning`, `store_compiled`,
  `compile`, `curate`, `promote`, and semantic search operate here.

The existing bridge, `VaultStore.LoadExternalPages()` (commit 89244b0),
loads non-`disk-local` store pages into the vault's in-memory index —
but it runs exactly **once**, during background startup indexing
(main.go `deferredIndex`). Verified by live reproduction against
current `main`:

| Step (single server session) | Result |
|---|---|
| `store_compiled(tag="gateway", ...)` | stored (SQLite row + `.uf/dewey/compiled/gateway.md`) |
| `get_page(name="compiled/gateway")` | `page not found` |
| `list_pages()` | `[]` |
| `search(query="gateway")` | no results |
| `promote(page="compiled/gateway")` | **succeeds** (promote queries the store directly) |
| Server restart, `get_page(name="compiled/gateway")` | **succeeds** (startup `LoadExternalPages` ran) |

So #90 is a **live-refresh gap**: store writes that happen while the
server is running never reach the read tools. The same staleness
affects the MCP `index`/`reindex` tools (they write documents to the
store with no way to refresh the read index) and the `reload` tool
(it clears the in-memory index and re-reads files only, dropping all
previously loaded store pages until restart).

For #91, the MCP `promote` tool queries the store directly and works
when the server holds the store, but its error handling masks genuine
store failures as `Page not found` (tools/promote.go conflates
query errors with absence), and the CLI `dewey promote` command fails
with `another Dewey process is using this database` whenever the MCP
server holds the SQLite file lock (store.go acquires an exclusive,
non-blocking flock; main.go then continues without persistence, so a
second dewey instance silently serves a degraded, store-less view).

Both issues share one root cause — the vault read layer and the
internal store are not consistently bridged — and are fixed together
here. The resulting PR closes #90 and #91.

## What Changes

1. **Live store read-through in the vault backend.** `vault.Client`
   read paths fall back to (or merge in) the persistent store when a
   page is not in the in-memory file index: `GetPage`,
   `GetPageBlocksTree`, and `GetBlock` fall back on miss;
   `GetAllPages` merges store-only pages; `FullTextSearch` merges a
   store-side block search; `FindBlocksByTag` generalizes its
   existing learning-only store fallback to all store-only pages.
   Lookups are live (no mid-session caching of store reads), so pages
   first stored during a session are visible immediately with their
   latest committed content. Known limitation: a page hydrated into the
   in-memory index at startup keeps in-memory precedence, so a mid-session
   re-store of that page serves the hydrated copy until the next reload
   (scoped to `compiled/*` re-stores; invalidation wiring is rejected in
   design D2).

2. **`reload` keeps store pages.** The `reload` MCP tool re-runs the
   external-page load after re-reading vault files instead of
   dropping every store-backed page until restart.

3. **Promote reliability and diagnosability (#91).** `promote`
   distinguishes "page not found" from underlying store errors,
   resolves internal page names the way agents call them (exact name,
   then `compiled/{tag}`, then `learning/{name}` — resolved decision:
   shorthand resolution is included; when the exact name misses and
   the input contains no `/`, promote tries `compiled/{name}` then
   `learning/{name}`, first existing wins),
   and gains regression tests covering learning and compiled pages.

4. **CLI promote lock error names the holder.** When `dewey promote`
   cannot open the store because another process holds the lock, the
   error includes the PID and command recorded in `.uf/dewey/dewey.lock`
   and points the user at the MCP `promote` tool as the supported
   write path while a server is running. The lock protocol itself is
   unchanged (out of scope below).

5. **Hardening found during investigation.** `store_compiled` gains a
   nil-store guard (currently panics when a server degraded to
   store-less mode), and the shared "requires persistent storage"
   error message hints at lock contention as a likely cause.

## Capabilities

### New Capabilities

- `unified-content-serve`: the MCP read tools `get_page`,
  `get_page_blocks_tree`, `get_block`, `list_pages`, `search`, and
  `find_by_tag` serve a unified, live view of vault files and internal
  store pages (learnings, compiled articles, external-source and
  knowledge-store pages), and promote locates internal store pages
  reliably. `query_properties` is not part of the merge in this change
  (deferred as the follow-up task noted in tasks.md).

### Modified Capabilities

- None (no `openspec/specs/` capability exists yet for this behavior;
  this change establishes `unified-content-serve`).

### Removed Capabilities

- None.

## Impact

**Affected code** (planning-level; see design.md for detail):

- `vault/vault.go` — read-path store fallbacks and merges; `Reload`
  re-runs external-page load; tag-search fallback generalization.
- `vault/vault_store.go` — single-page and merged read helpers
  extracted from `LoadExternalPages`; block-content search support.
- `store/store.go` — new parameterized block-content search query
  (and any name-set filtering helpers the merges need).
- `tools/promote.go` — error clarity, name resolution.
- `tools/compile.go` — nil-store guard on `StoreCompiled`.
- `cli.go` — promote lock-error diagnostics.
- Tests alongside each file; in-memory SQLite (`:memory:`) and
  `t.TempDir()` throughout.

**Behavioral guarantees:**

- All 37 inherited graphthulhu MCP tools continue to work identically
  when no store is configured (the fallback is a no-op with a nil
  `VaultStore`) — Neighborhood Rule.
- When a store is configured, read tools return the union: vault file
  pages take precedence on name collision; store-only pages are
  surfaced with their store provenance (tier, source, properties).
- Write guards are preserved: store-sourced pages remain read-only to
  vault write tools. The in-memory `readOnly` flag covers hydrated pages,
  and page-level write tools additionally resolve the guard from the store
  itself (a synchronous existence check before the create-if-missing
  branch), so store-backed pages served by the live read-through cannot be
  shadowed by a silently created vault file.
- MCP `promote` remains the supported write path while a server is
  running; the CLI lock behavior is unchanged.

**Explicitly out of scope:**

- Multi-process write coordination or lock-protocol redesign (shared
  locks, queueing, WAL-based ownership). The exclusive flock
  intentionally prevents concurrent write corruption (store.go T059);
  redesigning it is a data-integrity change that deserves its own
  change. This change only makes the failure diagnosable.
- `get_references` / `GetPageLinkedReferences` for store-only pages
  (backlink-index population for store pages). The tool is
  DataScript-gated for Logseq; noted as future work in design.md.
- Semantic search — already store-backed and working (it is the
  inconsistency callout in #90's impact section).
- `dewey serve`'s silent degrade-to-no-store behavior on lock
  acquisition failure (only the error-message hint improves).

## Constitution Alignment

Assessed against the Unbound Force org constitution.

### I. Autonomous Collaboration

**Assessment**: PASS

All changes stay behind the existing `backend.Backend` MCP contract —
no new runtime coupling, no direct function calls between heroes.
Agents retrieve learnings and compiled articles through the same
self-describing MCP tools (`get_page`, `search`, `list_pages`) they
already use, closing the artifact loop: store → retrieve → promote.
Provenance (tier, source, properties) travels with every store-backed
result, so outputs remain self-describing.

### II. Composability First

**Assessment**: PASS

Dewey remains independently installable and usable with zero new
dependencies (pure-Go SQLite and existing packages only). The bridge
is strictly optional: with no `.uf/dewey/` workspace the vault client
behaves exactly as before (graphthulhu-compatible in-memory mode),
preserving graceful degradation.

### III. Observable Quality

**Assessment**: PASS

Store-backed reads return the same machine-parseable shapes as vault
reads, carrying tier/source/properties provenance. The CLI promote
lock error becomes actionable (identifies the holding PID and command
from `dewey.lock` and names the supported alternative), replacing an
opaque flock error. The health tool's `dewey.persistent` /
`dewey.sources` fields continue to make index state auditable, and
this change makes the read view consistent with what they report.

### IV. Testability

**Assessment**: PASS

Every requirement has test scenarios using in-memory SQLite
(`:memory:`), `t.TempDir()` vaults, and the existing
`mockBackend`/tool test patterns — no external services, no network.
The live read-through design (no mid-session caching) makes behavior
deterministic: write-then-read sequences are assertable without
sleeps or invalidation hooks. Verified against a manual MCP stdio
reproduction; that repro is captured as GIVEN/WHEN/THEN scenarios in
the delta spec.
