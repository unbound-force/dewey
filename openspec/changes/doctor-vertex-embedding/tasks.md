## 1. Diagnostics

- [x] 1.1 `cli.go`: `printVertexEmbeddingChecks`; `vertex` case in
  the Embedding Layer switch; drop the FR-017 shared-path comment
- [x] 1.2 `cli_test.go`: configured and misconfigured Vertex
  embedding (no Ollama mention, no `ollama pull`)

## 2. Verification

- [x] 2.1 CI parity: build, vet, `go test -race -count=1
  -coverprofile=coverage.out ./...`, gaze crap gate, golangci-lint
- [x] 2.2 `/uf.review-council` (OpenCode), draft PR
- [x] 2.3 Verify constitution alignment: Composability First
  (Ollama stays optional), Autonomous Collaboration (diagnostic
  output only), Observable Quality (accurate provider
  diagnostics), Testability (isolated temp-vault tests, no
  external services)
- [x] 2.4 File website docs-sync issue for the doctor Embedding
  Layer output change (unbound-force/website, `docs` label;
  scoped to doctor output per the website#249 precedent,
  cross-reference website#113) — filed as website#389
