## 1. Input and validation

- [ ] 1.1 `types/tools.go`: add `Evidence []string`, `Validity`,
  `Supersedes` to `StoreLearningInput` with jsonschema descriptions
- [ ] 1.2 `tools/learning_temporal.go`: `validateTemporalInput`
  (validity enum; evidence bounds, trim, dedupe) and tests

## 2. As-of pinning

- [ ] 2.1 `tools/learning_temporal.go`: `resolveAsOf(resolver)` and
  `productionAsOfResolver(vaultPath)` (git top-level basename + HEAD,
  2 s timeout); tests with injected resolvers and a temp git repo
- [ ] 2.2 `tools/learning.go`: `Learning` gains an injectable
  `asOfResolver`; `NewLearning` defaults it to production

## 3. Properties and frontmatter

- [ ] 3.1 `tools/learning_temporal.go`: `temporalProperties` and
  `renderTemporalFrontmatter` (quoted strings, evidence block
  sequence); tests
- [ ] 3.2 `tools/learning.go`: `StoreLearning` merges the properties,
  writes the frontmatter after `tier:`, returns the fields

## 4. Supersession

- [ ] 4.1 `tools/learning_temporal.go`: `setFrontmatterKeys`
  (replace-or-insert keys in place, body untouched); tests
- [ ] 4.2 `tools/learning.go`: resolve and validate the target before
  writing; after insert, update target properties and backup file;
  `supersedes_warning` on partial failure; tests

## 5. Re-ingestion

- [ ] 5.1 `main.go`: `learningFrontmatter` fields; re-ingest restores
  them via one helper; tests for full and legacy files

## 6. Docs and verification

- [ ] 6.1 `server.go` tool description; README `store_learning`
  section
- [ ] 6.2 CI parity: `go build ./...`, `go vet ./...`,
  `go test -race -count=1 -coverprofile=coverage.out ./...`,
  `gaze crap --max-crapload=48 --max-gaze-crapload=45`,
  `golangci-lint run`
- [ ] 6.3 `/uf.review-council` (OpenCode), draft PR
