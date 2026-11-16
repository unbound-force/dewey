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

  No Phase 1 or Phase 2 task is [P]-eligible: Phase 1
  tasks all edit agents.go in dependency order, and
  Phase 2 tasks are three-way coupled — the embedded
  templates, tracked mirrors, and shape guards must
  land together for the test suite to go green (design
  Risk 4), with the guard edits all in cli_test.go.
  Phase 3 tasks touch disjoint surfaces (CI commands,
  AGENTS.md/README.md, GitHub issues) and are [P].

  Phase checkpoints run the exact Build/Vet/Test steps
  from .github/workflows/ci.yml. The Gaze threshold
  gate and golangci-lint run at the Phase 3 CI-parity
  gate. The Phase 1 checkpoint is build/vet only: the
  parity and shape tests are deliberately red between
  the template edit and Phase 2 (tracked mirrors and
  guards move with the templates), so the full suite
  is the Phase 2 checkpoint, not Phase 1's.
-->

## 0. Preconditions

- [ ] 0.1 Confirm the change artifacts under
  `openspec/changes/scaffold-bash-terminal-denies/` are committed and
  pushed on `opsx/scaffold-bash-terminal-denies` (Spec Commit Gate),
  and that the working tree is clean before implementation starts.

## 1. Embedded Agent Templates (agents.go)

- [ ] 1.1 In `deweyAgentDefinitions["curator.md"]`, insert the exact
  10-line terminal-deny block from design D1 after the
  `"dewey gate auto-answer": allow` line (agents.go:47) and before the
  closing `---`: the 3-line comment ("Terminal catch-all denies,
  placed last so they win over every / allow on any prefix: each shell
  metacharacter is a / chaining, redirection, or substitution
  vector.") then the seven denies in canonical order `*>*`, `*<*`,
  `*|*`, `*;*`, `*&*`, `*$(*`, `` *`* ``, each `deny`. The
  `` "*`*": deny `` line uses the backtick splice from design D2 —
  close the raw string literal after `"*`, concatenate the
  interpreted string `"`"``, resume the raw literal with `*": deny`
  (the established `+ "`" +` technique from slash_commands.go).
- [ ] 1.2 In `deweyAgentDefinitions["distiller.md"]`, insert the same
  10-line block (design D1, same splice for `` *`* ``) after the
  `"gh issue comment": allow` line (agents.go:148) and before the
  closing `---`, so the map reads: `"*": deny`, the three allows,
  then the seven terminal denies as the final entries.
- [ ] 1.3 Update the `deweyAgentDefinitions` doc comment
  (agents.go:11-12) with the exact replacement text from design D2,
  documenting the `` "*`*" `` terminal-deny key as the single
  backtick exception spliced via Go string concatenation. Keep the
  existing no-backtick rule and the quotes-for-identifiers guidance
  for everything else.

**Checkpoint (Phase 1)**: `go build ./...` and `go vet ./...` must
pass. The full test suite is deliberately deferred to the Phase 2
checkpoint: the tracked mirrors and shape guards move with the
templates (design Risk 4), so parity and shape tests are expected red
here — mid-sequence failures are coupling, not regressions.

## 2. Tracked Mirrors and Shape Guards (mirrors, cli_test.go)

- [ ] 2.1 Regenerate the tracked mirrors per design D3: build the
  binary (`go build`), run `dewey init --vault .` at the repository
  root, and verify `git status` / `git diff` shows exactly two changed
  files — `.opencode/agents/curator.md` and
  `.opencode/agents/distiller.md`, each carrying only the 10-line
  bash-block insertion plus the recomputed `dewey:scaffold` marker
  digest line — while the slash commands, workflow example, `.uf/dewey/`
  config files, `.opencode/dcp.jsonc`, and `.gitignore` are unchanged.
  The run must log `updated agent` for exactly the two agent paths.
- [ ] 2.2 Extend `assertCuratorFrontmatterShape` in cli_test.go
  (design D4): grow the ordered, length-exact `wantBash` list
  (cli_test.go:5891-5897) from two entries to the nine-entry sequence
  — `{"*", "deny"}`, `{"dewey gate auto-answer", "allow"}`, then the
  seven terminal denies in canonical order, each `deny` — and update
  the length-mismatch error message to name the new shape. YAML
  comment lines are invisible to the `yaml.Node` walk, so `bashPairs`
  counts exactly the nine mapping entries.
- [ ] 2.3 Extend `assertDistillerFrontmatterShape` in cli_test.go
  (design D5): add the ordered last-7 assertion — the mapping's final
  seven `bashPairs` entries are exactly the seven terminal deny keys
  in canonical order, each `deny` — and add `gh issue comment` to the
  allow-preservation loop beside the two startup gates (DI-FR-004).
  Keep the existing `"*": deny` check.
- [ ] 2.4 Doctored-fixture cases executing the new rejection arms
  (design D4/D5): add two rows to
  `TestCuratorAgent_GuardsRejectDoctoredDocuments` — a terminal deny
  deleted (expects the `permission.bash has 8 entries` length
  mismatch) and an allow moved after the terminal denies (expects the
  ordered entry mismatch) — and one compact distiller doctored case
  (drop one terminal deny from a copy of the distiller document,
  assert the last-7 arm reports the mismatch) using the shared
  `problemRecorder` infrastructure.

**Checkpoint (Phase 2)**: `go build ./...`, `go vet ./...`,
`go test -race -count=1 ./...` must pass with the regenerated mirrors
and extended guards in place — including the unmodified parity tests
(`TestInitCmd_ScaffoldsCuratorAgentFromMap`,
`TestInitCmd_FirstInitWritesVerifiableMarkers`) and the prompt-text
allowlist parity tests.

## 3. Verification and Documentation

- [ ] 3.1 [P] CI-parity gate derived from `.github/workflows/ci.yml`:
  `go mod download`, `go build -v ./...`, `go vet ./...`,
  `go test -race -v -count=1 -coverprofile=coverage.out ./...`, the
  Gaze threshold gate with the repository's flags
  (`gaze crap --format=json --coverprofile=coverage.out
  --max-crapload=48 --max-gaze-crapload=45 ./...`), and golangci-lint.
  Record results in the PR body.
- [ ] 3.2 [P] Documentation gate: AGENTS.md — add a Recent Changes
  entry for scaffold-bash-terminal-denies (terminal metacharacter
  denies closing every scaffolded agent bash map, the backtick splice
  exception in agents.go, recomputed mirror markers, the extended
  shape guards) and verify the Key Patterns sentences describing the
  agents' bash allowlists remain accurate (the allows are unchanged;
  no rewrite expected); README.md — the curator paragraph (~line 790)
  "the only shell command it may run is `dewey gate auto-answer`"
  stays true, extend it with one clause noting the terminal
  metacharacter denies so the scaffolded security shape is documented.
  GoDoc is covered by task 1.3.
- [ ] 3.3 [P] Website documentation sync issue per the AGENTS.md
  website-sync rule (user-facing `dewey init` output changed): file
  `gh issue create --repo unbound-force/website --label docs` titled
  "docs: sync dewey init agent scaffold documentation" describing the
  seven terminal metacharacter denies now ending every scaffolded
  curator/distiller `permission.bash` map, the explanatory comment,
  and the affected `content/docs/` pages (the dewey init and agent
  scaffold documentation). Record the issue number here:
  website#____.
- [ ] 3.4 Constitution alignment verification per the proposal's four
  PASS assessments and the design's Constitution Alignment section —
  confirm the permission map remains the self-describing enforcement
  boundary with no runtime coupling (I), no new dependency and inert
  YAML for non-OpenCode consumers (II), marker digests keep the
  output verifiable and the guards make the security shape
  machine-checkable (III), and the pure-content change is verifiable
  with the standard library testing package (IV) — and record the
  confirmation in the PR body.

**Checkpoint (Phase 3)**: re-run `go build ./...`, `go vet ./...`,
`go test -race -count=1 ./...` with the documentation in place.
