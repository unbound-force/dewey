# Tasks: fix-cli-search-backend

## 1. Rewrite Search Command

- [x] 1.1 Rewrite `newSearchCmd()` in `cli.go` to use the vault backend instead of the Logseq HTTP client:
  - Resolve vault path from `--vault` flag (already on root command) or `OBSIDIAN_VAULT_PATH` env var (same logic as `initObsidianBackend` at `main.go:170-176`)
  - Return a clear error if vault path is not set
  - Create `vault.New(vaultPath)`, call `vc.Load()` to index local `.md` files
  - Build the search index via `vc.BuildBacklinks()` (which triggers `rebuildSearchIndex`)
  - If `.dewey/graph.db` exists, open store, attach via `vault.WithStore(s)`, and call `vs.LoadExternalPages(vc)` to include external content
  - Call `vc.FullTextSearch(ctx, query, limit)` to get `[]backend.SearchHit`
  - Print results (page name + matching content) to stdout
  - Close store on exit via `defer s.Close()`
- [x] 1.2 Remove the `client` package import from `cli.go` if it is no longer used by any other function (check `newJournalCmd` and `newAddCmd` — they may still use it) — **KEPT**: `client` is still used by `newJournalCmd()` and `newAddCmd()`

## 2. Update Tests

- [x] 2.1 Update `TestSearchCmd_NoQuery` in `cli_test.go` to verify the error message is unchanged
- [x] 2.2 Rewrite `TestSearchCmd_WithResults` in `cli_test.go` to create a temp vault with `.md` files, run the search command with `--vault` pointing to the temp dir, and verify results are returned
- [x] 2.3 Rewrite `TestSearchCmd_NoResults` in `cli_test.go` to create a temp vault, search for a non-matching query, and verify the "no results" error

## 3. Verification

- [x] 3.1 Run `go build ./...`, `go vet ./...`, `go test -race -count=1 ./...` — all must pass
- [x] 3.2 Manual verification: run `dewey search "Implementation" --vault .` in the dewey repo and confirm results are returned
