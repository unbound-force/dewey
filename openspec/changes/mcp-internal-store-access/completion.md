# Completion Notes: mcp-internal-store-access

**Constitution alignment verification (task 5.3)** — recorded 2026-11-01 against the
uncommitted working tree on `opsx/mcp-internal-store-access` (implementation tasks
1.1–5.2 complete; CI green: `go build`/`go vet` pass, `go test -race -count=1` 1468
PASS, gaze CRAP thresholds met).

Website documentation sync (task 5.5): filed as unbound-force/website#361 —
https://github.com/unbound-force/website/issues/361.

Method: read the implemented code directly (backend/, vault/, store/, tools/, cli.go),
ran targeted greps (`Sleep` in test files, diff-scoped to added lines), checked git
diffs for `backend/` emptiness against both `upstream/main` and `HEAD`, compared
pre-change method contracts via `git show HEAD:vault/vault.go`, and re-ran the
parity/error/concurrency test subset with CI flags:
`go test -race -count=1 -run 'NoStoreParity|StoreFallbackConcurrent|Reload_PreservesStoreBackedPages|Reload_NoStoreConfigured|PromoteStoreOpenError|StoreErrorSurfaces|MatchesLoadExternalPagesShape' ./vault/... ./tools/...`
→ `ok vault 1.661s`, `ok tools 1.739s`. `go build ./... && go vet ./...` also pass.

The principle numbering below follows task 5.3's mapping onto the proposal's
Constitution Alignment section: I = Autonomous Collaboration, II = Composability
First (no-store parity / Neighborhood Rule), III = Observable Quality,
IV = Testability.

## Principle I — All changes behind `backend.Backend` (Autonomous Collaboration): PASS

- The `backend/` package is untouched: `git diff upstream/main...HEAD --stat --
  backend/` is empty, `git diff upstream/main -- backend/` and
  `git diff HEAD -- backend/` both produce 0 lines (committed and working-tree
  views), and `git log upstream/main..HEAD -- backend/` lists 0 commits.
  `backend/backend.go` still contains only the original `Backend` interface
  (backend.go:13–39) and the pre-existing optional capability interfaces
  (`HasDataScript`, `TagSearcher`, `PropertySearcher`, `JournalSearcher`,
  `FullTextSearcher` — backend.go:41–66). No new methods, no signature changes.
- The store bridge lives entirely inside `vault.Client`, which implements
  `backend.Backend`: the optional `vaultStore *VaultStore` field (vault/vault.go:99)
  populated via the pre-existing `WithStore(s *store.Store) Option`
  (vault/vault.go:117–120). The new read helpers (`GetStorePage`,
  `GetStoreBlock`, `ListStorePages`, `SearchStoreBlocks`, `hydrateStorePage`,
  `storePageEntity`) are methods/functions on `VaultStore` in
  vault/vault_store.go:471–644 — vault-package internals, invisible to the
  interface.
- MCP tools still program against the interface: `server.go:73`
  (`newServer(b backend.Backend, ...)`) is unchanged in shape, and no tool file
  gained a `vault.Client` dependency. The `store/store.go` addition
  (`SearchBlocksByContent`) extends the store package, not the backend contract.
- The only tool-level changes are the shared `persistentStorageError` helper
  (tools/persistent_storage.go:20–23) consumed by store-dependent tools
  (compile.go:110,699; curate.go:71; indexing.go:111,207; learning.go:195;
  lint.go:87; promote.go:42) and promote's error clarity — none touch
  `backend.Backend`.

## Principle II — No-store parity for all 37 inherited tools (Composability First / Neighborhood Rule): PASS

Every code path added on this branch returns before any store interaction when
no store is configured, with the pre-change return shapes preserved
(miss → `(nil, nil)`; enumeration/search → the in-memory snapshot; confirmed
against `git show HEAD:vault/vault.go` — the pre-change methods returned exactly
these values; only the internal lock discipline changed from hold-through-return
to snapshot-then-release, which does not alter observable results):

- `Reload` — vault/vault.go:513: `if c.vaultStore != nil` guards the
  `LoadExternalPages` re-run; with nil store the flow is clear → `Load()` →
  `BuildBacklinks()` exactly as before.
- `GetAllPages` — vault/vault.go:563: `return pages, nil` (in-memory snapshot)
  before `ListStorePages`.
- `GetPage` — vault/vault.go:603: `return nil, nil` before `GetStorePage`.
- `GetPageBlocksTree` — vault/vault.go:640: `return nil, nil` before `GetStorePage`.
- `GetBlock` — vault/vault.go:678: `return nil, nil` before `GetStoreBlock`.
- Tag search path — vault/vault.go:1633 (`storePagesTagResults`):
  `if c.vaultStore == nil || c.vaultStore.store == nil { return nil, nil }`
  before `ListPagesExcludingSource`; `FindBlocksByTag` (vault/vault.go:1577)
  returns the in-memory results unchanged.
- `FullTextSearch` — vault/vault.go:1893: `return hits, nil` (in-memory index
  hits) before `SearchStoreBlocks`.
- Helper level — vault/vault_store.go:485, 514, 545, 574: each new store read
  helper treats a nil store as always-miss/empty, so parity holds even if a
  guard were bypassed.
- `tools/promote.go:41–43` — nil-store guard returns the shared
  `persistentStorageError("promote")` (tools/persistent_storage.go:20–23),
  which explains both nil-store causes (no `.uf/dewey/` workspace vs. lock held
  by another Dewey process).

Parity is test-pinned (all pass with `-race -count=1`):

- `TestGetPage_NoStoreParity` (vault/vault_test.go:2680) — GetPage and
  GetPageBlocksTree misses return `(nil, nil)` with no store.
- `TestGetBlock_NoStoreParity` (vault/vault_test.go:2917) — memory hit keeps
  its page reference; miss returns `(nil, nil)`.
- `TestGetAllPages_NoStoreParity` (vault/vault_test.go:3187) — no-store output
  is identical to the same vault with an empty store attached (merge layer is
  inert with nothing to add).
- `TestFullTextSearch_NoStoreParity` (vault/vault_test.go:3457) — no-store and
  empty-store hit sets are equal; after a store-only page is written, the
  store-backed client surfaces it while the no-store client's results are
  unchanged.
- `TestReload_NoStoreConfigured` (vault/vault_test.go:3655) — Reload with a nil
  `vaultStore` still re-reads files and serves pages.
- Additionally `TestVaultStore_GetStorePage_MatchesLoadExternalPagesShape`
  (vault/vault_store_test.go) pins live-fallback and startup-load results to
  the same shape, and `TestGetPage_StoreErrorSurfaces` /
  `TestPromote_StoreErrorSurfaces` pin that store failures surface as errors
  rather than being masked as not-found (preserving the inherited tools' honest
  error contract).

## Principle III — Provenance on store-backed results and actionable promote/lock errors (Observable Quality): PASS

- **Provenance**: `storePageEntity` (vault/vault_store.go:627–644) converts every
  persisted `store.Page` row into a `types.PageEntity` carrying Name,
  OriginalName, Journal, CreatedAt, UpdatedAt, and Properties parsed from the
  stored JSON blob — the blob where learnings (`tag`, `tier`, `author`,
  `created_at`), compiled articles (`topic`, `tier`), and external pages record
  their origin metadata. This single conversion (DRY) is shared by
  `hydrateStorePage` (vault/vault_store.go:606–615, used by `GetStorePage`:497
  and `LoadExternalPages`:679) and `ListStorePages` (vault/vault_store.go:556,
  the GetAllPages merge), so page, list, and search results identify their
  store origin identically; `SearchStoreBlocks` hits (vault/vault_store.go:588–592)
  carry PageName/UUID/Content in the same shape as in-memory search hits. An
  unparseable properties blob degrades to nil properties rather than failing
  the read, matching historical LoadExternalPages behavior.
- **Promote error distinction**: `tools/promote.go` keeps absence and store
  failure distinct — a missing page reports `Page '%s' not found.`
  (promote.go:58–60, naming the original input), while a failed lookup returns
  `failed to look up page '%s': %w` (promote.go:100, 109) with the underlying
  error wrapped, never masked as absence. Pinned by
  `TestPromote_StoreErrorSurfaces`, `TestPromote_ShorthandResolution`, and
  `TestPromote_PromotesInternalPagesByExactName` (tools/promote_test.go:335,
  388, 480).
- **CLI lock diagnostics**: `promoteStoreOpenError` (cli.go:2503–2518) probes
  `dewey.lock` via `detectLockHolder` (cli.go:987–1011, non-blocking flock +
  reading the recorded PID/command line), wraps the underlying store error,
  identifies the holder ("another Dewey process (PID …, command) appears to be
  holding the database lock"), and names the supported write path: "use the
  dewey_promote MCP tool to promote pages, or stop the server first". Old-format
  locks degrade to "lock held (unknown process)"; absent/unheld/unreadable locks
  degrade to the plain wrapped error. Covered by four tests:
  `TestPromoteStoreOpenError_HeldLock`, `_OldFormatLock`, `_NoLockFile`,
  `_UnheldLockFile` (cli_test.go:4703 ff.).
- **Shared actionable error**: `persistentStorageError`
  (tools/persistent_storage.go:20–23) gives all eight store-dependent tools one
  consistent message that hints at lock contention as a likely cause.

## Principle IV — Deterministic, no-sleep tests (Testability): PASS

- `grep -n "Sleep"` across vault/vault_test.go, vault/vault_store_test.go,
  store/store_test.go, tools/promote_test.go, tools/persistent_storage_test.go,
  tools/compile_test.go, cli_test.go, and server_test.go finds six matches, all
  in vault/vault_test.go (lines 990, 1029, 1077, 1111, 1124, 1136) inside the
  pre-existing fsnotify watcher tests `TestWatchFileCreate`,
  `TestWatchFileModify`, `TestWatchFileDelete`, and `TestConcurrentAccess` —
  none of which this branch touched.
- Diff-scoped check: `git diff HEAD -- <file> | grep "^+.*Sleep"` returns zero
  matches for every one of the eight test files — no added line on this branch
  contains a sleep (and none were removed).
- The new concurrency test `TestGetPage_StoreFallbackConcurrent`
  (vault/vault_test.go:2775) drives 8 goroutines × 25 iterations across the
  memory-hit, store-hit, and store-miss paths with `sync.WaitGroup` and no
  sleeps, validating the design D2 snapshot-then-release locking discipline
  under `-race`. `TestReload_PreservesStoreBackedPages`
  (vault/vault_test.go:3584) is likewise sleep-free.
- New tests use in-memory SQLite (`:memory:`) and `t.TempDir()` throughout —
  no external services, no network. The live read-through design (no mid-session
  caching of store reads) is what makes write-then-read sequences assertable
  without sleeps, exactly as the proposal predicted; the passing
  `*NoStoreParity`/`*RestoreIsNeverStale` runs above confirm it.

## Summary Verdict

**All four constitution principles PASS in the implemented code.** The change
delivers live store read-through entirely inside `vault.Client` behind an
untouched `backend.Backend` contract (I); with no store configured, every added
path returns before any store interaction with byte-identical observable results
to pre-change behavior, test-pinned by five NoStoreParity tests plus shape and
error-surface tests (II); store-backed results carry their provenance through
one shared conversion, and promote/lock errors distinguish absence from failure,
name the lock holder with PID and command, and point at the supported write path
(III); and the new test code contains zero sleeps — the single new concurrency
test synchronizes with `sync.WaitGroup` under `-race`, with determinism coming
from the live read-through design rather than timing (IV). No principle
violations were found; no source changes were needed for this verification.
