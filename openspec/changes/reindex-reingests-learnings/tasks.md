<!--
  [P] marks tasks eligible for parallel execution.
-->

## 1. Implementation

- [x] 1.1 `main.go`: extract `restoreFileBackedPages` from serve's
  deferred index; serve calls it in the same place (design D1).
- [x] 1.2 `cli.go`: `newReindexCmd` calls `restoreFileBackedPages`
  after `IndexDocuments` and before `recordIndexBuild`; long help and
  GoDoc updated (design D2, LR-FR-003).

## 2. Tests

- [x] 2.1 `cli_test.go`: `TestReindexCmd_RestoresFileBackedPages` —
  learning page (source, `as_of` property, blocks) and knowledge-store
  page present after reindex; sources indexed; stamp recorded. Fails
  before 1.2.
- [x] 2.2 Existing `TestReIngestLearnings_*`, `TestReIndexKnowledgeStores_*`
  and `TestReindexCmd_*` pass unchanged.

## 3. Documentation

- [x] 3.1 README `dewey reindex`, AGENTS.md CLI table and File-Backed
  Learnings.
- [x] 3.2 Website sync issue in `unbound-force/website` (`dewey
  reindex` behavior) — opened with the PR, after human go-ahead.

## 4. Verification

- [x] 4.1 Constitution alignment: proposal PASS assessments hold — no
  new flags or config (II), same log lines (III), real-command test
  against a temp vault (IV).
- [x] 4.2 CI parity: `go build ./...`, `go vet ./...`,
  `go test -race -count=1 ./...`.
- [ ] 4.3 Review council (`/uf.review-council`) — not available in
  this Claude Code session; to run before merge.
