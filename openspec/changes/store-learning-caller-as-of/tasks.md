<!--
  [P] marks tasks eligible for parallel execution.
-->

## 1. Implementation

- [x] 1.1 `types/tools.go`: optional `AsOf` on `StoreLearningInput`.
- [x] 1.2 `tools/learning_temporal.go`: `callerAsOfPattern`,
  `validateCallerAsOf`; `prepareTemporal` trims, validates and prefers
  the caller value, falling back to `resolveAsOf` (design D1-D3).
- [x] 1.3 `server.go`: `store_learning` description mentions the
  caller pin.

## 2. Tests

- [x] 2.1 `tools/learning_temporal_test.go`: `TestValidateCallerAsOf`
  (accepted and rejected shapes, including option injection and URLs);
  `TestStoreLearning_CallerAsOfOverridesDefault` (result, properties
  and frontmatter carry the trimmed pin; resolver not called; default
  unchanged without a pin); `TestStoreLearning_CallerAsOfWithoutResolver`;
  "bad as_of" case in `TestStoreLearning_InvalidTemporalInputWritesNothing`.

## 3. Documentation

- [x] 3.1 README Temporal Validity and tool table, AGENTS.md Store
  Learning API, GoDoc.
- [x] 3.2 Website sync issue in `unbound-force/website` (new MCP tool
  parameter) — opened with the PR, after human go-ahead
  (unbound-force/website#437).

## 4. Verification

- [x] 4.1 Constitution alignment: proposal PASS assessments hold —
  optional field, default unchanged (II); malformed pins rejected (III);
  pure validator + injected resolver tests (IV).
- [x] 4.2 CI parity: `go build ./...`, `go vet ./...`,
  `go test -race -count=1 ./...`.
- [ ] 4.3 Review council (`/uf.review-council`) — not available in
  this Claude Code session; to run before merge.
