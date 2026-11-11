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

  Phase checkpoints run the exact Build/Vet/Test steps from
  .github/workflows/ci.yml (see each checkpoint). The Gaze
  threshold gate and MegaLinter run only at the Phase 4.1
  CI-parity gate, which is the sole full CI-parity checkpoint.
-->

## 0. Preconditions

- [x] 0.1 Confirm the change artifacts under
  `openspec/changes/curator-tool-allowlist/` are committed and
  pushed on `opsx/curator-tool-allowlist` (Spec Commit Gate), with
  `originating_issue: 192` recorded in the change's `.openspec.yaml`.

## 1. Curator Frontmatter Enforcement

- [x] 1.1 Edit `.opencode/agents/curator.md`: append to the frontmatter
  `tools:` block, after the existing five entries and with the denies
  grouped before the allows, exactly these entries — `"dewey_*": false`,
  `"replicator_*": false`, then `dewey_lint: true`,
  `dewey_list_pages: true`, `dewey_compile: true`, `dewey_curate: true`,
  `dewey_store_compiled: true`, `dewey_promote: true` (quoted keys for
  the glob entries; a bare `*` is a YAML alias marker). Do not change
  the existing entries (`read: true`, `bash: true`, `write: false`,
  `edit: false`, `webfetch: false`), the `permission:` block, or the
  Markdown body. The broad `"dewey_*"` deny MUST precede the six allow
  entries (design D2).
- [x] 1.2 Mirror the edited file byte-for-byte into the
  `deweyAgentDefinitions["curator.md"]` raw string literal in
  `agents.go` (the new entries contain no backticks, so the raw literal
  form is unaffected). The tracked file and the map entry MUST stay
  byte-identical so `TestInitCmd_ScaffoldsCuratorAgentFromMap` stays
  green.

**Checkpoint** (exact ci.yml Build/Vet/Test steps):
`go build -v ./... && go vet ./... && go test -race -v -count=1
-coverprofile=coverage.out ./...`

## 2. Mechanical Verification

- [x] 2.1 Add `TestCuratorAgent_FrontmatterToolAllowlist` to
  `cli_test.go`: split the tracked `.opencode/agents/curator.md`
  frontmatter (`splitFrontmatter`) and parse the `tools:` mapping in
  entry order with `gopkg.in/yaml.v3` `yaml.Node` (already a
  dependency; preserves key order — a hand-rolled line parser in the
  style of `gate.go`'s `frontmatterValue` is acceptable if preferred).
  Assert: `"dewey_*"` is `false` and appears before every
  `dewey_*`-matching `true` entry; exactly the six expected
  `dewey_<tool>` keys carry `true` (`lint`, `list_pages`, `compile`,
  `curate`, `store_compiled`, `promote`); no other `dewey_*`-matching
  key is `true`; the built-in five entries are unchanged; every
  non-`dewey` server key configured in `opencode.json` (parse its `mcp`
  map keys) has a `<key>_*: false` entry; and the `permission:` block
  still carries its expected entries (`question: allow`; `task`,
  `edit`, `write`, `patch`, `webfetch` denied; the nested `bash` map
  with `"*": deny` preceding the single `dewey gate auto-answer`
  allow). Run the same shape assertions against the
  `deweyAgentDefinitions["curator.md"]` map entry.
- [x] 2.2 Add `TestCuratorAgent_AllowlistParity` to `cli_test.go`:
  collect the frontmatter `dewey_*` `true` keys, strip the `dewey_`
  prefix, and assert set-equality with `agentAllowlistEntries(body)`
  over the tracked file's body (reuse the existing extractor and
  `sameStringSet`), so the prompt-text allowlist and the frontmatter
  list cannot drift apart in either direction.
- [x] 2.3 Confirm the existing guards stay green with the new
  frontmatter: `TestInitCmd_ScaffoldsCuratorAgentFromMap` (byte
  identity of tracked, map entry, and scaffolded copy — the acceptance
  criterion's byte-identical test, now covering the new shape
  transitively) and `TestDeweyDistill_CuratorBindingAndHumanGates`
  (command references and prompt-text allowlist). Extend either test
  only if the new frontmatter reveals a gap; do not weaken any existing
  assertion.

**Checkpoint** (exact ci.yml Build/Vet/Test steps):
`go build -v ./... && go vet ./... && go test -race -v -count=1
-coverprofile=coverage.out ./...`

## 3. Documentation

- [x] 3.1 [P] Update `AGENTS.md`: add a Recent Changes bullet for
  `curator-tool-allowlist` describing the frontmatter-enforced
  deny-all-then-allow-six Dewey MCP allowlist, the other-server denies,
  and the parity tests; update any text describing the curator's MCP
  surface (the Server-side synthesis bullet and the
  curator-agent-human-gates bullet) so it no longer reads as
  prompt-only policy; state the server-key coupling — the glob keys
  assume the `dewey` MCP server key from `opencode.json`, and a repo
  using a different key must adjust the globs.
- [x] 3.2 [P] Update `README.md`: extend the curator agent permission
  model section with the frontmatter tool-glob enforcement (the deny
  entry, the six allows, the other-server denies) and the server-key
  coupling note.
- [x] 3.3 [P] Reconcile the website documentation: check whether the
  prior change's open issues (unbound-force/website#371 docs, #376
  tutorial, #377 blog) still cover the curator's permission model; if
  any is open, extend it by comment with the frontmatter enforcement
  and the server-key coupling; only if they are closed (or do not cover
  the curator's tool boundary) file
  `gh issue create --repo unbound-force/website --label docs` titled
  `docs: sync dewey curator tool allowlist documentation` describing
  what changed and which `content/docs/` pages need updating. Do not
  create a duplicate.

  **Outcome (2026-11-11): all three prior issues were open and were
  extended by comment — no duplicate filed.** docs
  unbound-force/website#371
  ([comment](https://github.com/unbound-force/website/issues/371#issuecomment-6802159535)),
  tutorial unbound-force/website#376
  ([comment](https://github.com/unbound-force/website/issues/376#issuecomment-6802159380)),
  blog unbound-force/website#377
  ([comment](https://github.com/unbound-force/website/issues/377#issuecomment-6802159568)).
  Each comment records the frontmatter enforcement (deny-all-then-allow-six,
  other-server denies) and the server-key coupling.

**Checkpoint** (exact ci.yml Test step):
`go test -race -v -count=1 -coverprofile=coverage.out ./...`

## 4. Verification

- [x] 4.1 Run the full CI-parity gate, deriving every command from
  `.github/workflows/ci.yml` and
  `.github/workflows/mega-linter.yml` (this is the sole full
  CI-parity checkpoint): `go build -v ./...`, `go vet ./...`,
  `go test -race -v -count=1 -coverprofile=coverage.out ./...`,
  `gaze crap --format=json --coverprofile=coverage.out
  --max-crapload=48 --max-gaze-crapload=45 ./...` (exit non-zero on
  regression), and the MegaLinter go flavor locally via
  `npx mega-linter-runner --flavor go` (gofmt/goimports,
  golangci-lint, markdownlint, yamllint, gitleaks). Failures block
  the task.
- [x] 4.2 Perform the manual acceptance matrix and record results:
  under the installed host with the new curator frontmatter, an
  allowlisted Dewey MCP call (`dewey_lint`) succeeds; a
  non-allowlisted Dewey MCP call (`dewey_delete_page`) is refused by
  the frontmatter tool flags — not merely declined per the prompt; a
  `replicator_*` MCP call is refused; both human-gate questions remain
  askable; `dewey gate auto-answer` still runs (the bash carve-out is
  unaffected). This matrix is also the design-D4 glob verification: if
  the glob keys prove inert (the non-allowlisted call is NOT refused),
  execute the fallback — replace the `"dewey_*"` deny with explicit
  `false` entries for every registered Dewey MCP tool except the six
  (derive from `registeredMCPToolNames`), add the lockstep test that
  fails when a new MCP tool is registered without a curator deny
  entry, and re-run this matrix; if neither form is honored, stop and
  escalate the gap rather than shipping an inert boundary.
- [x] 4.3 Verify constitution alignment (principles I–IV remain PASS);
  confirm `git status --short .opencode/agents/` shows only
  `curator.md` modified with no other agent's tool flags, permission
  rules, mode, or command binding changed; confirm the curator's
  `permission:` block is unchanged from the prior change (the diff
  touches only the `tools:` block); confirm no change to
  `slash_commands.go`, `.opencode/commands/`, or `opencode.json`.
- [x] 4.4 Stretch (optional, per issue #192): either implement the
  gate-side server-key verification — extend `dewey gate auto-answer`
  in `gate.go` to parse the project OpenCode configuration's `mcp` map
  and refuse (fail-closed, naming the configured and expected keys)
  when a server invoking the `dewey` binary is keyed as something other
  than `dewey`, with `gate_test.go` fixture tests — or record the
  deferral decision here. Completing this task means one of the two
  outcomes is recorded; the deferral is acceptable per the issue's
  wording and MUST NOT be silently dropped.

  **Deferral decision (2026-11-11): deferred.** The stretch implementation
  is not taken, for four reasons: (1) it is optional per issue #192's
  wording; (2) the change's functional requirements (CA-FR-009..012) are
  fully satisfied without it; (3) extending `gate.go` would add scope to a
  file this change does not otherwise touch, after the spec review already
  approved the current shape — the task-2.1 shape test already fails closed
  when the repository-root `opencode.json` `mcp` map is missing, empty, or
  lacks the `dewey`/`replicator` keys, covering this repo's own
  configuration; (4) the acceptance matrix found the installed dewey
  binary (mise v3.5.3) predates the `gate` subcommand itself (#152 not yet
  released), so gate-side verification would not be runtime-testable in
  this environment. Follow-up: file as a separate change when the gate
  subcommand ships in a release and a mis-keyed server is a live concern.

**Checkpoint** (exact ci.yml Test step):
`go test -race -v -count=1 -coverprofile=coverage.out ./...`

## 5. Review

- [ ] 5.1 Run `/uf.review-council` and resolve findings until all
  reviewers return APPROVE; re-run the Phase 4 checks after any fix.
