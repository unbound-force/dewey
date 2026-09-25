<!--
  [P] marks tasks eligible for parallel execution.
  Add [P] when a task: (a) touches different files from
  other [P] tasks in the group, (b) has no dependency
  on prior tasks in the group, (c) can safely execute
  without ordering constraints.
  Do NOT add [P] when tasks modify the same file --
  parallel workers will cause merge conflicts.
  Tasks without [P] run sequentially first, then [P]
  tasks run in parallel.
-->

## 1. Regression Coverage

- [x] 1.1 Add a table-driven `buildCompiledArticle` history summary regression test in `tools/compile_test.go` with em dash, CJK, emoji, and exactly 80-rune cases.
- [x] 1.2 Assert for every case that the complete article is valid UTF-8 and that the history row contains the explicit expected summary, including correct ellipsis behavior.
- [x] 1.3 Confirm the test fails when the truncation is temporarily changed back to byte slicing, restore the rune-safe implementation, and confirm the test passes.

## 2. Verification

- [x] 2.1 Format `tools/compile_test.go` with `gofmt` and run `go test -race -count=1 ./tools/...`.
- [x] 2.2 Run the CI-equivalent checks from `.github/workflows/ci.yml`: `go mod download`, `go build -v ./...`, `go vet ./...`, `go test -race -v -count=1 -coverprofile=coverage.out ./...`, install Gaze, and run the configured `gaze crap` threshold gate.
- [x] 2.3 Run the repository's MegaLinter-equivalent validation identified by the pre-flight workflow and resolve any findings without weakening protected gates.
- [x] 2.4 Verify the final diff remains test-only, preserves Composability First and Observable Quality, requires no README, AGENTS.md, GoDoc, or website documentation update, and satisfies the proposal's Testability assessment.

## Execution Checklist

- [x] Step 0: Startup Cleanup
- [x] Step 1: Branch Safety Gate
- [x] Step 2: Resumability Detection
- [x] Step 3: Clarify (Step 1)
- [x] Step 4: Plan (Step 2)
- [x] Step 5: Tasks (Step 3)
- [x] Step 6: Spec Review (Step 4) -- iteration: 3/3
- [x] Step 7: Implement (Step 5) -- phase: 2/2, batch: 0/0, workers: 0/0
- [x] Step 8: Code Review (Step 6) -- iteration: 1/3
- [x] Step 9: Retrospective (Step 7)
- [x] Step 10: Demo (Step 8)

<!-- scaffolded by uf v0.17.0 -->

<!-- spec-review: passed -->

<!-- code-review: passed -->
