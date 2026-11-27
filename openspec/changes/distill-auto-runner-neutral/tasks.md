<!--
  [P] marks tasks eligible for parallel execution.
  Add [P] when a task: (a) touches different files from
  other [P] tasks in the group, (b) has no dependency
  on prior tasks in the group, (c) can safely execute
  without ordering constraints.
  Do NOT add [P] when tasks modify the same file —
  parallel workers will cause merge conflicts.

  Phase checkpoints run the exact Build/Vet/Test steps from
  .github/workflows/ci.yml.
-->

## 0. Preconditions

- [ ] 0.1 Commit the change artifacts under
  `openspec/changes/distill-auto-runner-neutral/` on
  `opsx/distill-auto-runner-neutral` before implementation (Spec Commit
  Gate); working tree clean.

## 1. Gate (gate.go, gate_test.go)

- [ ] 1.1 Replace the supervisor.yaml probe with a single-path resolver
  per design D1 (absolute non-blank XDG wins; relative/blank XDG
  ignored; HOME otherwise) and `<dir>/config.json` for the injected
  `supervisorConfigDir` seam.
- [ ] 1.2 Replace the YAML `commands` lookup with the JSON `profiles`
  match per D2 (trim + strip leading `/`); fail-closed refusals per D3.
- [ ] 1.3 Drop the `Notice:` line and `supervisorUnavailableNotice` (D4);
  keep the no-op artifact; update the Fix text, GoDoc and cobra Long.
- [ ] 1.4 Tests: pass, absent, XDG precedence, relative XDG ignored,
  invalid JSON, profiles not an array, profile for another command,
  leading-slash command, repo-local never read, render without Notice.

**Checkpoint (Phase 1)**: `go build ./...`, `go vet ./...`,
`go test -race -count=1 ./...`.

## 2. Command, agent and workflow template

- [ ] 2.1 Embedded `dewey-distill-auto` command: drop `#N`, the
  dispatcher line and the issue-comment step; refusal stops and relays;
  note that a stray trailing argument is ignored.
- [ ] 2.2 Embedded `distiller` agent: remove `"gh issue comment": allow`
  and its justifying prose; update the startup-gate and widening-rule
  text.
- [ ] 2.3 Regenerate the tracked `.opencode/` copies (with scaffold
  markers) from the embedded payloads; update allowlist tests.
- [ ] 2.4 Delete `workflow_templates.go`, `workflow_template_test.go`,
  `.github/workflows/dewey-distill.yml.example`; remove the `dewey init`
  scaffolding step and the related assertions in `cli_test.go` and
  `scaffold_test.go`. Record the missing retirement path (D6).

## 3. Documentation and verification

- [ ] 3.1 README weekly-distill section and AGENTS.md: runner-neutral
  scheduling, supervisor JSON profile, no dispatcher references; reword
  dispatcher mentions in code comments.
- [ ] 3.2 `git grep -n -i gh-hud -- ':!specs' ':!openspec' ':!.uf'`
  returns nothing.
- [ ] 3.3 CI-parity gate from `.github/workflows/`: build, vet, race
  tests, Gaze threshold step, golangci-lint.
- [ ] 3.4 Constitution alignment verification (per proposal): no new
  dependency, verdict + artifact audit surface intact, tests use the
  existing seams only.
