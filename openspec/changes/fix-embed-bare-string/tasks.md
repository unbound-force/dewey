<!--
  [P] marks tasks eligible for parallel execution.
  Add [P] when a task: (a) touches different files from
  other [P] tasks in the group, (b) has no dependency
  on prior tasks in the group, (c) can safely execute
  without ordering constraints.
  Do NOT add [P] when tasks modify the same file —
  parallel workers will cause merge conflicts.
  Tasks without [P] run sequentially first, then [P]
  tasks run in parallel.
-->

## 1. Fix Embed Wire Format

- [x] 1.1 In `embed/embed.go`, change line 131 from `o.doEmbed(ctx, text)` to `o.doEmbed(ctx, []string{text})` so that `Embed()` always sends the array form to `/api/embed`.
  - File: `embed/embed.go`

- [x] 1.2 In `embed/embed_test.go`, add an assertion to `TestOllamaEmbedder_Embed` that verifies `req.Input` is `[]any` (JSON array), matching the pattern used in `TestOllamaEmbedder_EmbedBatch` at line 74. The test server handler should reject bare-string input.
  - File: `embed/embed_test.go`

## 2. Verification

- [x] 2.1 Run `go build ./...`, `go vet ./...`, and `go test -race -count=1 ./embed/...` to confirm the fix passes all existing and new tests.

- [x] 2.2 Verify constitution alignment: Composability (Dewey standalone, no new deps) and Testability (wire format tested in isolation via `httptest`).
<!-- spec-review: passed -->
<!-- code-review: passed -->
