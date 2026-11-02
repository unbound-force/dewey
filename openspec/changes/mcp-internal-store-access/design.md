# Design: mcp-internal-store-access

## Context

`dewey serve` wires two data layers into one MCP server:

- The **vault layer** (`vault.Client`, implements `backend.Backend`) —
  an in-memory index built from vault `.md` files. Every inherited
  read tool (`get_page`, `get_block`, `list_pages`, `search`,
  `find_by_tag`, `query_properties`, journal, analyze) operates on it.
- The **store layer** (`store.Store`, SQLite at
  `{vault}/.uf/dewey/graph.db`) — the durable index over everything:
  vault files (source `disk-local`), external sources (GitHub, web,
  code, knowledge stores), learnings (`learning/{identity}`), and
  compiled articles (`compiled/{tag}`). `store_learning`,
  `store_compiled`, `compile`, `curate`, `promote`, and semantic
  search operate on it.

The existing bridge is `VaultStore.LoadExternalPages()`
(vault/vault_store.go): during background startup indexing (main.go
`deferredIndex`), it loads every non-`disk-local` store page into the
vault's in-memory index as a read-only `cachedPage` (fields
`sourceID`, `readOnly` already exist for exactly this purpose). It
runs once. Verified against current `main` via MCP stdio
reproduction:

- `store_compiled` then `get_page` in one session → `page not found`
  (the startup load already ran on an empty store; nothing refreshes
  it afterward). Same for `list_pages` → `[]` and `search` → no
  results.
- After a restart, the same page is fully visible — the startup load
  works.
- MCP `promote` already queries the store directly and succeeds in
  the simple case; the #91 report is consistent with its error
  handling masking store errors as "Page not found" (tools/promote.go
  returns the not-found message for *any* `GetPage` error) and with
  promote calls reaching a degraded, store-less server instance
  (main.go logs a warning and continues without persistence when the
  flock is held).
- CLI `dewey promote` opens the store with an exclusive non-blocking
  flock (store.go, T059) and fails with "another Dewey process is
  using this database" while any server is running.
- Two adjacent defects found while investigating: the `reload` MCP
  tool clears the in-memory index and re-reads files only (dropping
  all store-backed pages until restart), and `store_compiled` has no
  nil-store guard (it would panic on a store-less server, unlike
  `compile`, `store_learning`, `promote`, `lint`, and `curate`).

The gap is a **live-refresh gap**, not a missing bridge: the bridge
exists but is one-shot.

## Goals / Non-Goals

### Goals

- Pages written to the internal store during a live session are
  readable immediately via `get_page`, `get_block`, `list_pages`,
  `search`, and `find_by_tag` (#90).
- `promote` reliably locates internal store pages and reports
  failures diagnosably (#91, MCP path).
- CLI `dewey promote` lock failures identify the lock holder and the
  supported alternative (#91, CLI diagnosability).
- `reload` preserves store-backed pages.
- Degraded (store-less) mode is panic-free with clear errors.
- All 37 inherited graphthulhu tools behave identically when no store
  is configured (Neighborhood Rule / constitution II).

### Non-Goals

- Redesigning the multi-process lock protocol (shared/exclusive
  locking, queued writers). The exclusive flock intentionally
  prevents concurrent write corruption (store.go T059). The supported
  write path while a server runs is MCP; CLI promote remains usable
  when no server is running. This change only makes the failure
  diagnosable.
- Backlink-index population for store-only pages
  (`GetPageLinkedReferences` / `get_references`). The vault backlink
  map is built from file content at load time; store pages parsed
  from markdown may carry `[[wikilinks]]` whose targets resolve
  through the merged enumeration, but the reverse index is not
  extended. `get_references` is DataScript-gated for Logseq anyway.
  Future work: build backlinks from `store.GetForwardLinks` /
  `GetBackwardLinks` for store-only pages.
- Changing `dewey serve`'s degrade-to-no-store startup behavior (the
  health tool already reports `dewey.persistent: false`).
- Semantic search — already store-backed and working.
- `query_properties` / `FindByProperty` store merge. The property
  scan iterates the in-memory index; extending it follows the same
  pattern as the tag merge and is deliberately deferred to keep this
  change surgical (noted in tasks as follow-up).
- Changing the `readOnly` write-guard semantics for store-sourced
  pages.

## Decisions

### D1 — Live read-through in `vault.Client`, not a composite backend

**Decision.** Implement the bridge inside `vault.Client` read
methods as store fallbacks/merges, reached through the existing
`c.vaultStore` reference. Do not introduce a new composite
`backend.Backend` wrapper.

**Rationale.** The codebase has already committed to this
architecture: `LoadExternalPages` populates the vault's own index
with store pages, `cachedPage` carries `sourceID`/`readOnly` for
them, write guards reject writes to them, and `FindBlocksByTag`
already reaches into the store from inside the vault client
(`storeLearningTagResults`). A composite backend would duplicate
that machinery (write routing, read-only guards, optional-interface
forwarding for `TagSearcher`/`PropertySearcher`/`JournalSearcher`/
`FullTextSearcher`/`HasDataScript`) while the vault client keeps the
same code for the file watcher and CLI paths. Every tool flows
through `backend.Backend` method calls, so fixing the methods fixes
all tools at once — including the CLI `dewey search` command, which
already wires `vault.New` + `LoadExternalPages` and inherits the
fallbacks for free.

**Alternatives rejected.**
- *Composite/decorator backend* — cleaner in isolation, but it
  re-implements guard and interface-forwarding logic that
  `vault.Client` already owns, and it leaves `vault.Client` (used
  directly by the CLI and the watcher) un-fixed.
- *Store-awareness in each tool* (`tools/navigate.go`, `search.go`,
  …) — violates DRY; 37 tools × duplicated merge logic; largest
  blast radius for the Neighborhood Rule.

### D2 — Live lookups, no mid-session caching of store reads

**Decision.** `GetPage`, `GetPageBlocksTree`, and `GetBlock` query
the store on every in-memory miss and do **not** insert the result
into `c.pages`. `GetAllPages`, `FullTextSearch`, and
`FindBlocksByTag` merge store results into their return values on
every call, again without caching.

**Rationale.** Caching hydrated pages (à la `LoadExternalPages`)
would require invalidation for every in-process store mutation —
`store_compiled` deletes and re-inserts the same page name,
`store_learning` creates pages, `index`/`reindex`/`curate` bulk-write
via `vault.IndexDocuments`, background curation writes
knowledge-store pages. Missing one path reintroduces exactly the
staleness bug this change fixes. Live lookups keep every read of a
store-only page current by construction — no code path can serve a cached
copy — and keep write paths untouched. Known limitation: a page hydrated
into `c.pages` at startup keeps in-memory precedence, so a mid-session
re-store of that page (e.g. `compiled/*` same-tag re-store) serves the
hydrated copy until the next reload; only store-only pages get
always-current semantics. The cost
is one indexed SQLite query per miss (`pages.name` is the primary
lookup; `blocks.uuid` is indexed) and one extra table scan for
`GetAllPages` — negligible at observed store sizes (~1.3k pages,
sub-millisecond name queries with `modernc.org/sqlite` in-process).

The startup `LoadExternalPages` stays: it pre-warms the search index
and backlink map and remains the reason reads work today after a
restart.

**Locking discipline.** Store queries MUST NOT run while holding
`c.mu`. The read methods take `RLock` for the in-memory lookup,
release it, then query the store and merge. This avoids holding the
vault mutex across SQLite I/O and any lock-ordering hazard between
`c.mu` and the store's single connection.

**Alternatives rejected.**
- *Hydrate-on-miss into `c.pages`* — makes scan-based tools
  (`FindByProperty`, journal search) see store pages for free, but
  introduces the invalidation problem and stale-update behavior for
  re-stored pages (`store_compiled` same-tag re-store).
- *Event-driven refresh hooks in every write tool* — requires
  threading a refresher into `Learning`, `Compile`, `Indexing`,
  `Curate` (which today know only the store); any missed path
  regresses. This is the fragility the issue demonstrates.

### D3 — Merge semantics: vault wins, dedupe by name

**Decision.** Merged enumerations include a store page only when its
lowercased name is absent from the in-memory file index. `GetPage`
family consults memory first; only a miss reaches the store. The
store-side merge set reuses `ListPagesExcludingSource("disk-local")`
— the same predicate `LoadExternalPages` uses — so vault-mirrored
rows never surface twice.

**Rationale.** Vault files are the source of truth for local
content; the store's `disk-local` rows are their persisted mirror
(written by the indexing pipeline). Name-keyed dedupe is
deterministic and matches the vault client's existing
`lowerName`-keyed map. This also means external-source pages
(GitHub/web/code/knowledge stores) become visible to read tools —
the same fix class as the issue, and consistent with the documented
intent that indexed sources are searchable via MCP tools (AGENTS.md,
knowledge stores).

**Assumption recorded.** The local vault is persisted under source
ID `disk-local` (established by `initObsidianBackend` /
`NewVaultStore(s, vp, "disk-local")`). If multi-disk-source setups
ever index paths outside the vault root, the name-dedupe still
prevents duplication; only truly foreign same-named pages could
collide, resolved in the vault's favor.

### D4 — Store-side block search: new parameterized query

**Decision.** Add `store.Store.SearchBlocksByContent(query string,
limit int) ([]Block, error)` — a case-insensitive `LIKE` containment
match on `blocks.content`, parameterized, capped by `limit`, joining
page names. `FullTextSearch` merges those hits (skipping hits whose page
name is present in the in-memory index, regardless of whether that page
surfaced among the current query's results — a membership check against
the full index rather than the result set, so a limit-truncated in-memory
result cannot duplicate a page the store also returns) and respects the
caller's limit.

**Rationale.** The store has no full-text index; `LIKE` over
`blocks.content` is the same brute-force class as the vault's
brute-force fallback path in `tools/search.go` and is adequate for
issue-scale stores. No FTS5 virtual tables (schema migration risk,
CGO-free constraint already satisfied by `modernc.org/sqlite` but
FTS5 adds migration surface for marginal gain here). SQL safety:
parameterized queries only, per project convention.

### D5 — Tag-search generalization

**Decision.** Generalize `storeLearningTagResults` into a
store-pages tag fallback: enumerate store pages not present in the
in-memory index (same predicate as D3), match frontmatter
`tag`/`tags` properties (reuse `frontmatterHasTag`) **and**
block-content `#tags` (parse via `parser.Parse`, same as
`findTagInBlocks`), and merge `TagResult`s. The learning-only helper
becomes a special case of the generalized path.

**Rationale.** `find_by_tag` is used by decision tracking and by
agents to discover learnings; the current learning-only fallback
(vault/vault.go) already proves the pattern. Compiled pages carry
`topic` (not `tag`) in frontmatter — the spec scenario uses
`find_by_tag(tag="gateway")` against a compiled page whose
properties include `topic: gateway`. Resolved decision: `topic` IS
treated as a tag-equivalent property, source-scoped to pages whose
`source_id` is `compiled` — a stray `topic` property on a learning or
external page does not match. Rationale: `compile` clusters by
dominant tag and `topic` is the user-visible tag of a compiled
article.

### D6 — Promote: exact match, then prefix resolution; honest errors

**Decision.** Keep promote on the store (it already bypasses the
vault layer — correct). Change two things:
1. Split the error paths: `(nil, nil)` from `GetPage` → `Page
   '<name>' not found`; a non-nil error → surface
   `failed to look up page: <err>` so store failures are diagnosable
   (the current code reports both as "not found").
2. When the exact name misses and the input contains no `/`, try
   `compiled/{name}` then `learning/{name}`, first existing wins
   (resolved decision: shorthand resolution is included; #91 records
   agents calling both forms).

**Rationale.** `store_compiled` and `store_learning` results both
return the full page name, so exact matching remains primary;
resolution only rescues the shorthand calls recorded in #91. The
invariants (draft-only, target `validated`, `updated_at` refresh)
are untouched and covered by existing plus new regression tests.

### D7 — CLI promote lock diagnostics, not lock redesign

**Decision.** In `cli.go`'s promote command, when `store.New` fails,
read `.uf/dewey/dewey.lock` (best-effort; the lock file already
records `<pid> <command line>` per store.go) and print an error like:
`another Dewey process is using this database (holder: PID 1234
'dewey serve --vault .'). Use the promote MCP tool while a Dewey
server is running, or stop the server first.` Degradate gracefully
when the file is unreadable or empty. No change to the flock itself.

**Rationale.** The lock holder information is already recorded for
diagnostics; surfacing it converts an opaque failure into an
actionable one without touching the concurrency model (Non-Goal).
The error message also steers users to the intended API (MCP), which
this change makes reliable.

### D8 — Hardening: nil-store guards and message hint

**Decision.** Add the missing nil-store guard to `StoreCompiled`
(panic risk today) returning the same shared "requires persistent
storage" error used by sibling tools. Extend that shared message to
hint at lock contention: `... Configure --vault with a .uf/dewey/
directory, or another Dewey process may be holding the database
lock.` — applied consistently across the tools that embed it.

**Rationale.** The degraded mode (second server instance, lock held)
currently produces either confusing "page not found" errors or a
panic; the hint plus the health tool's `dewey.persistent: false`
make the state self-diagnosing (constitution III).

### D9 — Reload keeps store pages

**Decision.** `vault.Client.Reload()` re-runs the external-page load
after re-reading files: clear the in-memory index, `Load()` files,
then `LoadExternalPages(c)` (when a store is configured), then
`BuildBacklinks()`. The `reload` MCP tool in server.go needs no
change — it calls `Reload()`.

**Rationale.** Today `reload` silently shrinks the read view to
files-only until restart, which contradicts the tool's description
("Force a full vault re-index") and would regress the merged view
this change guarantees. Reusing `LoadExternalPages` keeps one code
path for the startup and reload flows (DRY).

## Risks / Trade-offs

- **Latency on misses.** Every `GetPage` miss for a genuinely absent
  page now costs one extra indexed SQLite query before returning
  not-found. Acceptable at issue-scale stores; the query is
  name-keyed and the connection is in-process. If stores grow by
  orders of magnitude, a negative-result cache can be added later —
  deliberately not now (D2).
- **`GetAllPages` merge cost.** `list_pages`/analyze tools pay one
  store scan per call (page rows only, no blocks). The analyze tools
  additionally hold a 30-second `graph.Cache`, so repeated graph
  queries do not re-merge within a window; that also means
  newly-stored pages can take up to 30s to appear in graph-overview
  results — documented behavior, not a defect (read tools other than
  analyze are immediate).
- **Behavioral surface grows.** `list_pages`/`search`/
  `graph_overview` now return store-only pages (external sources
  included), which is a visible output change for users who
  configured sources. This is the documented intent of unified
  content serve (commit 89244b0) and of AGENTS.md's knowledge-store
  claims; the delta spec pins it with scenarios. Rollback of any
  merge is per-method (each fallback is independent).
- **Store-less parity.** Every fallback guards on
  `c.vaultStore == nil`, so the in-memory mode (graphthulhu
  compatibility) is byte-for-byte unchanged; this is the Neighborhood
  Rule guarantee and is scenario-tested.
- **Concurrency.** Store reads happen outside `c.mu` (D2). The
  watcher mutates `c.pages` under `c.mu`; the merge snapshots the
  in-memory name set under `RLock`, then queries the store — a page
  indexed concurrently may momentarily be double-checked but dedupes
  by name on merge. No new writes to shared state; `-race` covers the
  interleavings in tests.
- **SQLite single-connection contention.** The store uses
  `SetMaxOpenConns(1)`; added read traffic serializes with indexing
  writes behind the 5s busy timeout. Read tools are interactive-rate;
  background indexing is bursty but bounded — same profile semantic
  search already produces today.

## Constitution Alignment

Carried from the proposal (all four principles PASS); the design
decisions that bear on each:

- **I. Autonomous Collaboration** — the fix is entirely behind the
  `backend.Backend` MCP contract (D1); agents close the
  store→read→promote artifact loop through existing self-describing
  tools.
- **II. Composability First** — nil-store guards make every addition
  a no-op without persistence (D2, D8); no new dependencies.
- **III. Observable Quality** — honest promote errors (D6),
  lock-holder diagnostics (D7), and the persistent-storage hint (D8)
  turn silent degradation into actionable output.
- **IV. Testability** — live lookups (D2) make write-then-read
  sequences deterministic without sleeps or invalidation hooks; all
  scenarios are testable with `:memory:` SQLite and `t.TempDir()`.
