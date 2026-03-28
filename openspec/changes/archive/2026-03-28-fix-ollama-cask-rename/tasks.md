## 1. Update GoReleaser Cask Dependency

- [x] 1.1 In `.goreleaser.yaml`, change `- cask: ollama` to `- cask: ollama-app` (line 37)

## 2. Update Documentation

- [x] 2.1 In `README.md`, update the Ollama install command from `brew install ollama` to `brew install --cask ollama-app` (line 352)

## 3. Verification

- [x] 3.1 Run `goreleaser check` to validate the GoReleaser config syntax
- [x] 3.2 Run CI-equivalent checks: `go build ./...`, `go vet ./...`, `go test -race -count=1 ./...`
- [x] 3.3 Verify constitution alignment: Composability First is maintained (Ollama remains an optional runtime dependency; the cask name change does not introduce new coupling)
