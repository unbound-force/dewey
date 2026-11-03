## 1. Diagnostics

- [x] 1.1 `cli.go`: `printVertexEmbeddingChecks`; `vertex` case in
  the Embedding Layer switch; drop the FR-017 shared-path comment
- [x] 1.2 `cli_test.go`: configured and misconfigured Vertex
  embedding (no Ollama mention, no `ollama pull`)

## 2. Verification

- [x] 2.1 CI parity: build, vet, `go test -race -count=1
  -coverprofile=coverage.out ./...`, gaze crap gate, golangci-lint
- [ ] 2.2 `/uf.review-council` (OpenCode), draft PR
