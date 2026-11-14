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

  No task in this change is [P]-eligible: Phase 1 tasks
  all edit gate.go in dependency order, Phase 2 tasks all
  edit gate_test.go, and Phase 3 tasks depend on both.

  Phase checkpoints run the exact Build/Vet/Test steps from
  .github/workflows/ci.yml. The Gaze threshold gate and
  golangci-lint run only at the Phase 3.1 CI-parity gate.
-->

## 0. Preconditions

- [ ] 0.1 Confirm the change artifacts under
  `openspec/changes/gate-config-sources-parity/` are committed and
  pushed on `opsx/gate-config-sources-parity` (Spec Commit Gate), and
  that the working tree is clean before implementation starts.

## 1. Gate Implementation (gate.go)

- [ ] 1.1 Add a candidate-construction helper per design D1, D4 and D6
  that returns the ordered, lexically de-duplicated candidates with
  their role (explicit, home, config directory, XDG): read
  `AUTO_QUESTION_CONFIG` and `OPENCODE_CONFIG_DIR` through `lookup`,
  trim, treat blank as unset, expand `~` and `~/...` against `$HOME`,
  refuse a value that is relative after expansion (message names the
  variable and value), and keep `resolvePluginConfigDirs` unchanged.
- [ ] 1.2 Use the helper in `evaluateAutoAnswerGate`: `ConfigFile` is
  the first candidate and `AdditionalConfigFiles` the rest; keep the
  injected `configDir` single-path seam (D5) and the registration
  scan's directory (D7) unchanged.
- [ ] 1.3 Refuse when the explicit (`AUTO_QUESTION_CONFIG`) candidate
  does not exist, naming the file, with a fix to point the variable at
  an existing plugin config or unset it (D4). Every other per-file
  condition keeps going through `resolvePluginAgents`.
- [ ] 1.4 Select `EffectiveAgents` and `DefaultApplied` by the plugin's
  merge rule (D3): the explicit file alone when present; otherwise the
  config-directory file when it defines `agents`, else the
  home-directory file; never the XDG file.
- [ ] 1.5 Update GoDoc on `gateOptions`, `evaluateAutoAnswerGate` and
  the new helper; keep `go vet` and `gofmt` clean.

**Checkpoint (Phase 1)**: `go build ./...`, `go vet ./...`,
`go test -race -count=1 ./...` must pass; every existing gate test
passes unmodified.

## 2. Acceptance and Edge Tests (gate_test.go)

- [ ] 2.1 Acceptance tests through `lookupEnv` with `t.TempDir()`
  trees: (a) config-directory file lists `curator`, clean home →
  REFUSED naming the config-directory file; (b) `AUTO_QUESTION_CONFIG`
  file lists `curator`, clean home → REFUSED naming it; (c)
  `AUTO_QUESTION_CONFIG` clean, home lists `curator` → REFUSED naming
  the home file (over-check, D2); (d) wildcard in the config-directory
  file → REFUSED.
- [ ] 2.2 Effective-agent tests (D3): (a) home and config-directory
  files with different lists → config-directory list; (b)
  config-directory file without `agents` → home list; (c)
  `AUTO_QUESTION_CONFIG` file with its own list → that list, verdict
  names it first then home, config directory and XDG; (d)
  `AUTO_QUESTION_CONFIG` file without `agents` → default with
  `DefaultApplied`.
- [ ] 2.3 Fail-closed and resolution tests (D4): relative
  `AUTO_QUESTION_CONFIG` → REFUSED; relative `OPENCODE_CONFIG_DIR` →
  REFUSED; missing `AUTO_QUESTION_CONFIG` file → REFUSED naming it;
  missing config-directory file → absent, PASS; `~/...` values expand
  against `$HOME`; blank values behave as unset; `OPENCODE_CONFIG_DIR`
  equal to `$HOME/.config/opencode` or to the XDG directory is checked
  once.
- [ ] 2.4 Run the full existing gate suite unmodified
  (`go test -race -count=1 -run
  'Test(GateAutoAnswer|RenderGateVerdict|StripJSONC|RunAutoAnswerGate)'
  .`); a failure means the implementation drifted, not that a test is
  stale.

**Checkpoint (Phase 2)**: `go build ./...`, `go vet ./...`,
`go test -race -count=1 ./...` must pass.

## 3. Verification and Documentation

- [ ] 3.1 CI-parity gate from `.github/workflows/ci.yml`:
  `go build ./...`, `go vet ./...`, `go test -race -count=1 ./...`,
  the Gaze CRAP threshold step with the repository's flags, and
  golangci-lint.
- [ ] 3.2 Documentation gate: README `dewey gate` section (the four
  sources and their order, the effective-agent rule, the new
  refusals); AGENTS.md Recent Changes entry; GoDoc covered by 1.5.
- [ ] 3.3 Website sync: extend unbound-force/website#371 with a comment
  describing the two new config sources and refusals (posted only with
  the maintainer's approval; record the comment id here).
- [ ] 3.4 Constitution alignment verification (per proposal); record
  the confirmation in the PR body.

**Checkpoint (Phase 3)**: re-run `go build ./...`, `go vet ./...`,
`go test -race -count=1 ./...` with the documentation in place.
