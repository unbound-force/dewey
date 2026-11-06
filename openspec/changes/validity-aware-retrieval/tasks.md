## 1. Results and inputs

- [x] 1.1 `types/tools.go`: `Validity`, `AsOf` on
  `SemanticSearchResult`; `IgnoreValidity` on both inputs; `Validity`
  on the filtered input
- [x] 1.2 `tools/semantic.go`: enrichment from page properties

## 2. Filter and ranking

- [x] 2.1 `tools/semantic_validity.go`: `validityWeight`,
  `rankByValidity`, `filterByValidity`, `semanticFetchLimit`
- [x] 2.2 `tools/semantic.go`: wire both tools

## 3. Tests

- [x] 3.1 `tools/semantic_validity_test.go`: weights, stable ranking,
  filter, fetch limit, enrichment, both tools end to end with a stub
  embedder

## 4. Docs and verification

- [x] 4.1 README semantic search rows
- [x] 4.2 CI parity: build, vet, `go test -race -count=1
  -coverprofile=coverage.out ./...`, gaze crap gate, golangci-lint
- [ ] 4.3 `/uf.review-council` (OpenCode), draft PR
