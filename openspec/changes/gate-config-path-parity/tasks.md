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

  Phase checkpoints (after Phase 1, Phase 2, and Phase 3) run
  the exact Build/Vet/Test steps from
  .github/workflows/ci.yml (see each checkpoint). The Gaze
  threshold gate and MegaLinter run only at the Phase 3.1
  CI-parity gate, which is the sole full CI-parity
  checkpoint.
-->

## 0. Preconditions

- [x] 0.1 Confirm the change artifacts under
  `openspec/changes/gate-config-path-parity/` are committed and
  pushed on `opsx/gate-config-path-parity` (Spec Commit Gate), and
  that the working tree is clean before implementation starts.

## 1. Gate Implementation (gate.go)

- [x] 1.1 Replace `resolveOpenCodeConfigDir` with a
  `resolvePluginConfigDirs(lookup)` function (or equivalent) that
  returns the ordered pair per design D1: `homeDir` =
  `filepath.Join(HOME, ".config", "opencode")` — `HOME` MUST be set,
  non-empty, and absolute, else the resolution fails; `xdgDir` =
  `filepath.Join(XDG_CONFIG_HOME, "opencode")` only when
  `XDG_CONFIG_HOME` is set, non-empty, absolute, and lexically
  different from `homeDir`; unset or empty `XDG_CONFIG_HOME` yields no
  XDG directory. A relative `XDG_CONFIG_HOME` fails resolution as
  today. Keep the failure wording for an unresolvable directory
  containing "config directory" (the existing
  `TestGateAutoAnswer_ConfigDirUnresolvableRefused` asserts it).
- [x] 1.2 Rework `evaluateAutoAnswerGate` per design D2 and D7: build
  the ordered candidate list
  (`homeDir/auto-question.jsonc`, then `xdgDir/auto-question.jsonc`
  when present); when `gateOptions.configDir` is injected, collapse
  to exactly that one candidate and skip environment resolution (D5);
  evaluate every candidate through the existing per-file logic
  (extract a per-file helper wrapping `resolvePluginAgents` per CS-010
  so `evaluateAutoAnswerGate` stays focused); refuse on the first
  offending or unverifiable file in the fixed home-then-XDG order;
  take `EffectiveAgents` and `DefaultApplied` from the
  home-directory candidate only; refuse when the home-directory path
  itself cannot be resolved (unset/empty/relative `$HOME`), including
  when `XDG_CONFIG_HOME` is set.
- [x] 1.3 Keep the advisory registration scan single-directory per
  design D6: pass the injected `configDir` when present, else the XDG
  directory when `XDG_CONFIG_HOME` is set and absolute, else the
  home-directory directory — the same directory the scan receives
  today in every reachable configuration. Do not widen the scan.
- [x] 1.4 Extend `gateVerdict` with `AdditionalConfigFiles []string`
  (ordered, after the primary) per design D4, and extend
  `renderGateVerdict` to print one `Also checked: <path>` line per
  additional file immediately after the `Config file:` line, on both
  PASS and REFUSED verdicts, each value passed through `safeValue`.
  Keep the `ConfigFile` field and the existing `Config file:` line
  unchanged.
- [x] 1.5 Update GoDoc on `gateOptions.configDir` (injected directory
  collapses evaluation to that single path) and
  `evaluateAutoAnswerGate` (checks the home-directory path always
  plus the XDG path when it differs; home-directory file is
  authoritative for the effective agent list). Verify `go vet` and
  `gofmt` stay clean.

**Checkpoint (Phase 1)**: run the exact Build/Vet/Test steps from
`.github/workflows/ci.yml` — `go build ./...`, `go vet ./...`,
`go test -race -count=1 ./...`. All three must pass before Phase 2
starts. The Gaze threshold gate and MegaLinter are deferred to the
Phase 3.1 CI-parity checkpoint.

## 2. Acceptance and Edge Tests (gate_test.go)

- [x] 2.1 Add the #191 acceptance tests using `lookupEnv` injection
  with `HOME` and `XDG_CONFIG_HOME` mapped to distinct `t.TempDir()`
  trees containing real fixture files (extend `newGateFixture` or add
  a sibling fixture helper that writes
  `<dir>/.config/opencode/auto-question.jsonc` and
  `<dir>/opencode/auto-question.jsonc`): (a) XDG config clean +
  home-directory config listing `curator` → REFUSED, reason names the
  home-directory file; (b) the reverse (XDG lists `curator`,
  home-directory clean) → REFUSED, reason names the XDG file; (c) both
  clean with distinct agent lists in the two fixtures (for example
  the home file listing `["autonomy", "builder"]` and the XDG file
  listing `["reviewer"]`) → PASS with `EffectiveAgents` equal to the
  home-directory list — distinct fixtures make authority leakage
  between the two files detectable; (d) home-directory config absent
  with a clean, parseable XDG config → PASS with `EffectiveAgents`
  the documented default `["autonomy"]`, `DefaultApplied` set, and
  both checked paths named (spec scenario "Effective agents come
  from the home-directory file" — the only arrangement that detects
  an implementation populating the effective list or default flag
  from the XDG file when the home file is absent). Each case asserts
  the verdict (or rendered output) names every checked file —
  acceptance criterion 4.
- [x] 2.2 Add edge tests: (a) `XDG_CONFIG_HOME` set to
  `$HOME/.config` lexically → exactly one file checked, no
  `Also checked` line (coincidence, single-path output preserved);
  (b) both files absent → PASS on the documented default with
  `DefaultApplied` set, both paths named; (c) unparseable XDG file
  with clean home-directory file → REFUSED; (d) wildcard `*` at the
  home-directory path with clean XDG file → REFUSED; (e) `HOME` unset
  while `XDG_CONFIG_HOME` is set to a valid directory → REFUSED
  (design D7 tightening); (f) `{env:AGENT}` interpolation resolving to
  `curator` in the home-directory file with a clean XDG file →
  REFUSED; (g) a stat of the XDG `auto-question.jsonc` failing for
  a reason other than file-not-found (for example a permission error
  on the XDG `opencode` directory) with a clean home-directory file
  → REFUSED, reason naming the XDG path.
- [x] 2.3 Add a `renderGateVerdict` test for the multi-file verdict:
  primary plus one additional file renders `Config file:` then
  `Also checked:` with the additional path, on pass and refusal
  shapes, and a control-character-laden additional path is sanitized.
- [x] 2.4 Run the full existing gate suite unmodified
  (`go test -race -count=1 -run
  'Test(GateAutoAnswer|RenderGateVerdict|StripJSONC|RunAutoAnswerGate)'
  .`) and confirm every pre-existing test still passes — they are the
  regression guard for single-path behavior (design D5, risk 4). Do
  not edit existing tests; a failure means the implementation
  drifted, not that the test is stale.

**Checkpoint (Phase 2)**: run the exact Build/Vet/Test steps from
`.github/workflows/ci.yml` — `go build ./...`, `go vet ./...`,
`go test -race -count=1 ./...`. All three must pass before Phase 3
starts. The Gaze threshold gate and MegaLinter are deferred to the
Phase 3.1 CI-parity checkpoint.

## 3. Verification and Documentation

- [x] 3.1 CI-parity gate (derive from `.github/workflows/ci.yml`,
  the source of truth): `go build ./...`, `go vet ./...`,
  `go test -race -count=1 ./...`, and the Gaze quality report with
  the repository's threshold flags. All must pass locally before the
  task is complete.
- [x] 3.2 Documentation gate: update the README `dewey gate` section —
  multi-path evaluation rule (home-directory path always, XDG path
  when it differs, home-directory file authoritative), the
  `Also checked:` verdict lines, and the tightened fail-closed
  description (unresolvable `$HOME` refuses even when
  `XDG_CONFIG_HOME` is set); add a Recent Changes entry to
  `AGENTS.md` describing gate-config-path-parity with the #191
  origin; assess GoDoc coverage of changed exported behavior (covered
  by 1.5).
- [x] 3.3 Website sync: extend the website tracking issue
  unbound-force/website#371 (comment) with the gate's path-parity
  behavior — multi-path evaluation, `Also checked:` verdict lines, and
  the new refusal condition — so the published `/dewey-distill` docs
  match the tool; capture the posted comment id in the task output
  for verification. No new docs issue (the gate docs live under that
  umbrella). Record the blog (website#377) and tutorial (website#376)
  determinations as scoped comments on those open sibling issues per
  the durable content-pipeline pattern (PR #188): both are below
  threshold — a bug fix to an existing subcommand whose
  default-config verdict output stays byte-identical — so neither a
  new blog post nor a tutorial change is required; capture each
  posted comment id in the task output.
  Posted: website#371 comment 6804606610 (gate path-parity addendum);
  website#376 comment 6804608869 (tutorial below-threshold);
  website#377 comment 6804609116 (blog below-threshold).
- [x] 3.4 Constitution alignment verification (per proposal): confirm
  Composability First (absent files at every checked path still pass
  on the default; no new dependency), Autonomous Collaboration
  (verdict remains the consumed artifact; exit-code contract
  unchanged), Observable Quality (every checked file named on pass
  and refusal, sanitized), and Testability (all new behavior covered
  by unit tests through the existing seams; existing suite
  unmodified and green). Record the confirmation in the PR body.

**Checkpoint (Phase 3)**: re-run the exact Build/Vet/Test steps from
`.github/workflows/ci.yml` — `go build ./...`, `go vet ./...`,
`go test -race -count=1 ./...` — with the documentation updates in
place. All three must pass before the change is declared complete.
The Gaze threshold gate and MegaLinter already ran at the Phase 3.1
CI-parity checkpoint.
