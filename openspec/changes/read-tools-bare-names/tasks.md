## 1. Resolution

- [x] 1.1 `vault/vault_store.go`: `findStorePage`; use it in
  `GetStorePage`
- [x] 1.2 tests: every scenario, plus error propagation and no-store
  parity

## 2. Docs and verification

- [x] 2.1 tool descriptions and README
- [x] 2.2 CI parity: build, vet, `go test -race -count=1
  -coverprofile=coverage.out ./...`, gaze crap gate, golangci-lint
- [x] 2.3 `/uf.review-council` (OpenCode), website docs issue, draft
  PR
