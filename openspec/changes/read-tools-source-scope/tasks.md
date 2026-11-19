<!--
  [P] marks tasks eligible for parallel execution.
-->

## 1. Implementation

- [x] 1.1 `types/tools.go`: add optional `SourceID` / `HasTag` to
  `SemanticSearchInput`, `SimilarInput`, `SearchInput`; `SourceID` to
  `FindByTagInput` (RS-FR-001..003).
- [x] 1.2 `tools/semantic.go`: `searchSimilarScoped` helper; route
  `SemanticSearch` and `Similar` through it (design D1, RS-FR-001).
- [x] 1.3 `tools/search_scope.go`, `tools/search.go`: `pageScope`,
  `scopeMatcher`, `scopedFetchLimit`; `Search.WithStore`; scope
  `search` (indexed and brute-force) and `find_by_tag` (tag searcher
  and DataScript); error without a store (design D2-D5, RS-FR-002..004).
- [x] 1.4 `server.go`: `NewSearch(b).WithStore(cfg.store)`; mention the
  scope in the four tool descriptions.

## 2. Tests

- [x] 2.1 `tools/search_scope_test.go`: semantic scope table, parity
  with `semantic_search_filtered`, `similar` scope (UUID and page
  query), indexed search scope table (case-insensitive page lookup,
  has_tag via properties and `#tag`, limit after scope), over-fetch,
  brute-force limit, no-store errors, `find_by_tag` via tag searcher
  and DataScript, `scopedFetchLimit`.

## 3. Documentation

- [x] 3.1 README tool tables, AGENTS.md tool matrix, GoDoc.
- [ ] 3.2 Website sync issue in `unbound-force/website` (MCP tool
  parameters changed) — opened with the PR, after human go-ahead.

## 4. Verification

- [x] 4.1 Constitution alignment: proposal PASS assessments hold —
  additive optional args (II), error rather than silent unscoped
  results (III), in-memory store + mock backends only (IV).
- [x] 4.2 CI parity: `go build ./...`, `go vet ./...`,
  `go test -race -count=1 ./...`.
- [ ] 4.3 Review council (`/uf.review-council`) — not available in
  this Claude Code session; to run before merge.
