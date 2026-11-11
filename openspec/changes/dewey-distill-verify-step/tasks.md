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

## 1. Verification tool core

- [x] 1.1 `types/tools.go`: add `VerifyCandidatesInput` with
  `pages []string` and `tags []string`.
- [x] 1.2 `tools/verify.go`: `Verify` struct, `NewVerify`, and
  candidate enumeration — `pages`/`tags` selectors (both is a
  validation error), default queue of tier `draft` pages under
  `learning/`, `compiled/`, and `knowledge/`, tag reading from
  page-name prefixes and the `tag` property with exact matching
  (delimiter required on name prefixes; `knowledge/` pages match
  only via the `tag` property; no substring matches),
  deduplication, and `skipped` entries for missing or non-draft
  requested pages (KV-FR-001, KV-FR-002).
- [x] 1.3 `tools/verify.go`: reuse `learningAnchors` and the
  `anchorChecker` seam for learning outcomes, and aggregate a
  compiled article's `sources` results with a deduplicated union
  of evidence, reporting each source identity with its individual
  result alongside the aggregate (KV-FR-003). Do not extract a
  shared pass runner from lint — see design D2's DRY decision.
- [x] 1.4 `tools/verify.go`: fixed result enum, `checked` /
  `skipped` / `failed` pass status with reason, error result
  (`IsError`) on failure with no candidate presented as verified
  from a failed pass, deterministic ordering, per-call timeout and
  pass budget, request-context cancellation, and the read-only
  guarantee (KV-FR-002, KV-FR-004, KV-FR-005).

**Checkpoint 1**: `go build -v ./... && go vet ./... &&
go test -race -count=1 ./...` passes.

## 2. Registration and wiring

- [x] 2.1 `server.go`: register `verify_candidates` and construct
  the handler with `cfg.store` and `cfg.vaultPath`; the input
  schema matches `VerifyCandidatesInput` (DE-007 layer 1)
  (KV-FR-001).
- [x] 2.2 `.opencode/commands/dewey-distill.md`: add the verify
  step between compile/curate and the review gate, renumber the
  later steps, and present per-candidate verification at the
  review gate, including the skipped/failed statement; preserve
  the `HUMAN_GATE:` marker and the no-promotion rule (KD-FR-104
  through KD-FR-107).
- [x] 2.3 `slash_commands.go`: update the `dewey-distill.md`
  scaffold entry so it stays byte-identical to the tracked command
  file.
- [x] 2.4 `.opencode/agents/curator.md` and `agents.go`: add
  `verify_candidates` to the Dewey MCP tool allowlist in the
  tracked agent file and in the `agents.go` scaffold mirror in the
  same commit, keeping the two byte-identical, and change nothing
  else (CA-FR-009, CA-FR-010). The extension carries the recorded
  human authorization in proposal.md ("Recorded Human
  Authorizations").

**Checkpoint 2**: `go build -v ./... && go vet ./... &&
go test -race -count=1 ./...` passes.

## 3. Tests

- [x] 3.1 `tools/verify_test.go`: enumeration and scope — `pages`,
  `tags`, both selectors rejected, default queue across
  `learning/`, `compiled/`, and `knowledge/` (including a draft
  `knowledge/` page and exclusion of a `curated`-tier one), exact
  tag matching (`auth` does not match `authentication`;
  `knowledge/` pages match only via the `tag` property),
  non-draft exclusion, deduplication, and missing/non-draft
  requested pages reported as `skipped` with a reason.
- [x] 3.2 `tools/verify_test.go`: anchor outcomes against a real
  temporary git repository (`anchors_ok`, `drifted`, `missing`,
  unknown pin) plus a fake `anchorChecker` for error paths, and
  compiled-article aggregation including an unresolvable source
  and per-source identity reporting (each source identity and its
  individual result present alongside the aggregate).
- [x] 3.3 `tools/verify_test.go`: degraded and failure modes
  (non-repository vault skipped, git failure `failed` plus
  `IsError`, cancellation, exhausted budget, and no candidate
  from a failed pass presented as verified — partial results ride
  the error as context only), deterministic repeat, and
  non-mutation of tier, validity, content, and files.
- [ ] 3.4 [P] `server_test.go`: registration surface and
  input-schema parity for `verify_candidates` (DE-007 layer 1).
- [x] 3.5 `tools/verify_test.go`: golden `checked` output shape —
  pass status, per-candidate fields, result enum, summary counts
  (DE-007 layer 2).
- [x] 3.6 `tools/verify_test.go`: round-trip non-mutation audit —
  verification leaves store content and tiers unchanged (DE-007
  layer 3).
- [ ] 3.7 [P] `cli_test.go`: tracked command file byte-identical to
  the scaffold entry, `.opencode/agents/curator.md` byte-identical
  to the `agents.go` scaffold mirror, both `HUMAN_GATE:` questions
  present, and the curator allowlist (in both agent copies)
  includes exactly `verify_candidates` as the one addition.

**Checkpoint 3**: `go test -race -v -count=1
-coverprofile=coverage.out ./...` passes.

## 4. Documentation and website sync

- [ ] 4.1 `README.md`: add `verify_candidates` to the MCP tools
  table and describe the verify step in the distillation and
  lint/anchor text. Exact edits: line 29 (tool count 50 → 51),
  line 765 (`/dewey-distill` pipeline description gains the verify
  step before the review gate), line 876 (`server.go — 50 tool
  registrations` → 51), line 904 (`types/tools.go — input types
  for all 50 tools` → 51).
- [ ] 4.2 `AGENTS.md`: update the tool/command references and
  Recent Changes with the verify step, the tool, and the allowlist
  extension. Exact edits: line 5 (tool count 50 → 51, category
  list gains verification), line 338 (`server.go — 50 tool
  registrations` → 51), line 343 (`tools/` package comment gains
  `verify`), line 371 (Server-side synthesis key pattern:
  `/dewey-distill` drives lint → inventory → scope gate →
  compile/curate → verify → review gate → batch promote).
- [ ] 4.3 [P] Extend the open website sync issue
  `unbound-force/website#371` (dewey-distill documentation) with a
  comment covering `verify_candidates` and the verify step, plus
  the curator allowlist addition — do not open a new issue; the
  tracked docs pages are the same.
- [ ] 4.4 [P] GoDoc: document every new exported symbol (`Verify`,
  `NewVerify`, `VerifyCandidatesInput`) with behavior and
  non-mutation guarantees.

**Checkpoint 4**: `gofmt -l .` reports nothing, `go vet ./...`
passes, and `go test -race -count=1 ./...` passes.

## 5. Verification and review

- [ ] 5.1 CI parity: run the exact checks from
  `.github/workflows/ci.yml` — `go build -v ./...`,
  `go vet ./...`, `go test -race -v -count=1
  -coverprofile=coverage.out ./...`, and
  `gaze crap --format=json --coverprofile=coverage.out
  --max-crapload=48 --max-gaze-crapload=45 ./...`.
- [ ] 5.2 Constitution alignment: confirm the four principles
  still hold — local-only git (Composability First), tool-call
  surface (Autonomous Collaboration), enum plus pass status
  (Observable Quality), and interface-backed tests (Testability).
- [ ] 5.3 Run `/uf.review-council` until APPROVE, then open the
  draft PR (`Closes #151`).
