## 1. Implementation

- [x] 1.1 `tools/curate.go`: persist `json.Marshal(props)` as
  `Properties` on insert and content-changed update in
  `IndexKnowledgeStore`; refresh `Properties` with the tier on the
  unchanged-content path (CKS-FR-002, design D3/D4).
- [x] 1.2 `main.go`: add `reIndexKnowledgeStores` (design D2) and call
  it from `deferredIndex` after `reIngestLearnings`, logging the count
  or a warning (CKS-FR-001, design D1).

## 2. Tests

- [x] 2.1 `tools/curate_test.go`:
  `TestAutoIndex_PersistsFrontmatterProperties`,
  `TestAutoIndex_UnchangedContentBackfillsProperties` (fail before 1.1).
- [x] 2.2 `main_test.go`: `TestReIndexKnowledgeStores_RecoversPages`
  (pages, source id, properties, `_index.md` skipped, second pass is a
  no-op), `_NoConfig`, `_InvalidConfig`, `_NilStore`.

## 3. Verification

- [x] 3.1 CI parity: `go build -v ./...`, `go vet ./...`,
  `go test -race -v -count=1 -coverprofile=coverage.out ./...`, and the
  Gaze threshold gate from `.github/workflows/ci.yml`.
- [x] 3.2 End-to-end on copies of two real vaults: a prototype build of
  this change (v3.7.0 base; it re-embedded on a properties-only
  difference, which D4 avoids), after
  `dewey reindex` and a server start, restored 461
  `cross-repo-contracts` pages and 347 pages of a source-less store,
  and `semantic_search_filtered` by validity returned the expected
  curated pages.
- [ ] 3.3 Review council (`/uf.review-council`).
