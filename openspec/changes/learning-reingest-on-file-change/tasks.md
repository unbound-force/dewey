<!--
  [P] marks tasks eligible for parallel execution.
  Add [P] when a task: (a) touches different files from
  other [P] tasks in the same group, (b) has no dependency
  on prior tasks in the group, and (c) can safely execute
  without ordering constraints.
  Do NOT add [P] when tasks modify the same file —
  parallel workers will cause merge conflicts.
  Tasks without [P] run sequentially first, then [P]
  tasks run in parallel.
-->

## 1. Implementation

- [x] 1.1 `main.go`: restructure `reIngestLearnings`'s existing-page
  branch into the three-branch hash comparison (design D1, D2): move
  the body-hash computation (`sha256(body)[:8]`, hex — unchanged input)
  above the `GetPage` check; hash match → refresh `Properties`,
  `Category` and `Tier` with one `UpdatePage` when any differs, no
  block or embedding work and no count (design D4); hash mismatch or
  empty stored hash → `DeleteBlocksByPage` → `UpdatePage`
  (`ContentHash`, `Properties`, `Tier: draft` with the forged-tier
  warning, `Category`, `SourceID`/`SourceDocID`) → `PersistBlocks` →
  `GenerateEmbeddings`, counted (LR-FR-001, LR-FR-002, design D5).
- [x] 1.2 [P] `tools/learning.go`: hash
  `strings.TrimSpace(input.Information)` in `store_learning` so rows it
  writes agree with the re-ingest body hash (design D3; LR-FR-002).

## 2. Tests

- [x] 2.1 `main_test.go`: `TestReIngestLearnings_ReingestsChangedFile`
  (edited body → new blocks, updated content hash, count 1, embedder
  called; fails before 1.1);
  `TestReIngestLearnings_UnchangedFileStaysCheap` (second pass over an
  unchanged file: count 0, block UUIDs and contents identical, a
  counting embedder fake records zero calls);
  `TestReIngestLearnings_FrontmatterOnlyChange` (frontmatter-only edit
  → properties refreshed, blocks untouched);
  `TestReIngestLearnings_DivergentHashConvergesOnce` (page stored with
  an empty hash → one full re-ingest, second pass cheap).
- [x] 2.2 `main_test.go`: update the two tests that pin the pre-change
  skip behavior. `TestReIngestLearnings_SkipsExisting`'s fixture gains
  the file's current content hash — assertions unchanged, so the test
  keeps verifying the unchanged-file skip it was written for.
  `TestReIngestLearnings_ExistingPageNotOverwritten` pins the superseded
  insert-only guarantee from the dewey-distill delta spec (design risk
  note; proposal Modified Capabilities): update it to pin the new
  convergence — count 1, tier `draft`, file-derived properties, blocks
  re-persisted from the file (LR-FR-001, including the drifted-tier
  clamp and the forged-tier warning).
- [x] 2.3 [P] `tools/learning_test.go`:
  `TestStoreLearning_HashMatchesFileBody` — store a learning whose
  information has leading/trailing whitespace and assert the stored
  page's `ContentHash` equals the hash of the trimmed file body as
  re-ingestion computes it (fails before 1.2).

- [x] 2.4 [P] `main_test.go` (review-fix tests, code-review findings): `TestReIngestLearnings_DriftedTierConvergesOnCheapPath`
  (page stored with a non-draft tier and a MATCHING content hash → the
  cheap path refreshes the tier to `draft` with no block or embedding work
  — LR-FR-001's drifted-tier scenario, previously uncovered); `TestReIngestLearnings_ToolStoredLearningStaysCheap`
  (joined write/read hash agreement: store via `tools`' `StoreLearning`
  with whitespace-padded information, then run `reIngestLearnings` →
  count 0 and zero embedder calls); extend the changed-file tests to
  assert `created_at` survival and embedding-row cleanup (design D4
  invariants).
- [x] 2.5 [P] `main_test.go` (iteration-2 review-fix test, code-review
  LOW finding): `TestReIngestLearnings_CategoryDriftConvergesOnCheapPath`
  (category as the sole cheap-path drift — pins the
  `existing.Category != fm.Category` arm of the refresh guard,
  LR-FR-002); extend `TestReIngestLearnings_ExistingPageConvergesToFile`
  with stale `SourceID`/`SourceDocID` fixture values asserted to converge
  on the changed path (pins main.go:1178-1179, LR-FR-001 — a stale
  SourceID would drop the page from `ListPagesBySource("learning")`).

## 3. Documentation

- [x] 3.1 Update the `AGENTS.md` File-Backed Learnings section (startup
  re-ingestion now covers changed files, not only orphaned ones),
  README.md's re-ingestion mentions (same broadening), and
  `reIngestLearnings`'s GoDoc. Record the website-sync decision in the
  PR body: no `unbound-force/website` issue — no command, flag, output
  format, MCP tool, or configuration field changed (AGENTS.md
  exemption for internal behavior).

## 4. Verification

- [x] 4.1 Constitution alignment: verify the proposal's four PASS
  assessments against the implemented code — learning files remain the
  durable record (I), no new dependencies or configuration (II), the
  startup count and log stay truthful (III), `reIngestLearnings` keeps
  its parameter-injected shape and the new tests run isolated with an
  in-memory store (IV) — and cite the evidence in the PR body.
- [x] 4.2 CI parity, commands derived from `.github/workflows/ci.yml`:
  `go build -v ./...`, `go vet ./...`,
  `go test -race -count=1 -coverprofile=coverage.out ./...`, and the
  Gaze threshold gate.
- [x] 4.3 End-to-end on a copy of a real vault: index, edit a learning
  file's body (simulated `git pull`), restart `dewey serve` and verify
  the page's blocks and content hash converge without `dewey reindex`;
  restart again and verify the pass is cheap (no embedding work); flip
  a frontmatter `validity` key and verify properties converge without
  block churn.
- [x] 4.4 Review council (`/uf.review-council`) until APPROVE.
