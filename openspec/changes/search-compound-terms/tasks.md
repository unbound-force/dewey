## 1. Index

- [x] 1.1 `vault/search_index.go`: `splitCompound`; index components
  in `indexBlocksLocked`
- [x] 1.2 tests: components indexed and removed with the page

## 2. Query

- [x] 2.1 `vault/search_index.go`: exact pass, then component pass
  for compound queries (dedupe, limit)
- [x] 2.2 tests: full, truncated, space-split IDs; exact-first
  ordering; dedupe; limit; non-compound query unchanged

## 3. Docs and verification

- [x] 3.1 README search row
- [x] 3.2 CI parity: build, vet, `go test -race -count=1
  -coverprofile=coverage.out ./...`, gaze crap gate, golangci-lint
- [ ] 3.3 `/uf.review-council` (OpenCode), draft PR
