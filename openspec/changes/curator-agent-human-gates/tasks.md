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
  threshold gate and MegaLinter run only at the Phase 5.1
  CI-parity gate, which is the sole full CI-parity checkpoint.
-->

## 0. Preconditions

- [x] 0.1 Confirm the change artifacts under
  `openspec/changes/curator-agent-human-gates/` are committed and
  pushed on `opsx/curator-agent-human-gates` (Spec Commit Gate).

## 1. Curator Agent Definition and Shipping

- [x] 1.1 Create `.opencode/agents/curator.md` in the repository's
  V1 agent format. Frontmatter: `description` (states the job and
  the boundary), `mode: subagent`, a `tools:` block (`read: true`,
  `bash: true`, `write: false`, `edit: false`, `webfetch: false` —
  `bash` is enabled because the startup gate is a CLI command and
  is narrowed in the permission block), and a `permission:` block
  (`question: allow`; `task`, `edit`, `write`, `patch`, `webfetch`
  each `deny`; `bash` as a nested map with `"*": "deny"` first,
  then exactly `"dewey gate auto-answer": "allow"` — no
  argument-bearing or prefix-widened allow rule). Body: the agent
  runs knowledge curation through exactly the allowlisted Dewey MCP
  tools (`lint`, `list_pages`, `compile`, `curate`,
  `store_compiled`, `promote`) and forbids every other Dewey MCP
  tool; persisted writes land only at those tools' destinations —
  `.uf/dewey/compiled/` for `compile`/`store_compiled`, the store
  database for `promote`, and each knowledge store's configured
  `path` for `curate`; no version-control
  operations; no source, test, documentation, or config edits; both
  mandatory gates are human-only and the startup gate must pass
  before any stage. Match the frontmatter shape of the existing
  V1 agents (`.opencode/agents/gaze-reporter.md`,
  `.opencode/agents/divisor-guard.md`).
- [x] 1.2 Create `agents.go` with a `deweyAgentDefinitions`
  `map[string]string` whose `"curator.md"` entry is byte-identical
  to the tracked `.opencode/agents/curator.md`, with GoDoc explaining
  that `dewey init` scaffolds these definitions only when
  `.opencode/` exists and never overwrites an existing file. Note:
  if the agent body uses backticks (for example naming MCP tools in
  inline code), build the Go string with concatenation around
  backtick literals the way `slash_commands.go` does — a Go raw
  string literal cannot contain a backtick.
- [x] 1.3 Edit `cli.go` inside the existing `.opencode/`
  composability guard: create `.opencode/agents/` when absent and
  write each `deweyAgentDefinitions` entry only when the target file
  does not exist, using `O_CREATE|O_EXCL` (not stat-then-write) so
  concurrent or racing invocations cannot overwrite. A write failure
  MUST fail `dewey init` with an error naming the target path; log a
  scaffold message only on successful create.
- [x] 1.4 Extend `cli_test.go` with
  `TestInitCmd_ScaffoldsCuratorAgentFromMap` (scaffolded bytes equal
  the map entry and the tracked file),
  `TestInitCmd_ScaffoldingSkipsExistingCuratorAgent` (sentinel bytes
  preserved), `TestInitCmd_CuratorAgentWriteFailureFailsInit`
  (read-only `.opencode/agents/` directory: init errors naming the
  path and does not report a successful scaffold), and a case
  asserting no `.opencode/agents/` directory is created when
  `.opencode/` is absent.

**Checkpoint** (exact ci.yml Build/Vet/Test steps):
`go build -v ./... && go vet ./... && go test -race -v -count=1
-coverprofile=coverage.out ./...`

## 2. Auto-Answer Gate Tool

- [x] 2.1 Create `gate.go` with the deterministic gate evaluation,
  factored for testability with injectable paths: resolve the
  OpenCode global config dir (`$XDG_CONFIG_HOME/opencode` when set,
  else `~/.config/opencode`), refusing when neither variable is set
  and when the directory cannot be stat'd for a reason other than
  not-found; read `auto-question.jsonc` there (absent file →
  documented default `agents` `["autonomy"]`); parse it as JSONC
  (tolerate comments and trailing commas without corrupting string
  contents); resolve `{env:VAR}` interpolation in `agents` entries
  against the process environment only when the whole entry is
  exactly the token, treating an unset variable or a partial
  interpolation as unverifiable; normalize every entry by trimming
  whitespace and case-folding before comparison; validate `agents`
  is an array of strings; refuse on `curator` or `*`; verify
  `.opencode/agents/curator.md` exists and declares `mode: subagent`,
  refusing when missing or unparseable; scan
  project configs (`.opencode/opencode.json[c]`, root
  `opencode.json[c]`) and the global `opencode.json[c]` for `plugin`
  (V1) and `plugins` (V2) arrays plus the discovery directories
  (`.opencode/plugins/`, global `plugins/`) for an
  `opencode-auto-question` or `auto-question` entry (final path
  segment match, leading `-` disable marker stripped) as advisory
  registration context with warnings for unreadable/unparseable
  scanned configs; return a structured verdict.
- [x] 2.2 Register a `dewey gate auto-answer` subcommand in `cli.go`
  following the existing subcommand pattern: render the tool's own
  verdict output (DE-003 — pass: `Auto-answer gate: PASS` plus the
  config file, effective agents list, default-applied flag, and
  registration status; refuse: `Auto-answer gate: REFUSED` plus the
  reason, the config file, and the fix), exit 0 on pass and 1 on
  refusal, no vault flag required. No `--json` flag.
- [x] 2.3 Create `gate_test.go` covering the full matrix over
  fixture directories (`t.TempDir()` with crafted configs): absent
  plugin config → pass on documented default; clean list → pass;
  `curator` listed → refuse (case-insensitive, including `Curator`
  and a whitespace-padded `" curator "`); `*` listed → refuse; file
  exists but unreadable (mode 000) → refuse; invalid JSONC → refuse;
  `agents` not a string array → refuse; `{env:...}` entry with the
  variable set to `curator` → refuse, unset → refuse; a partial
  interpolation inside a larger entry → refuse; config dir
  unresolvable (both `XDG_CONFIG_HOME` and `HOME` unset) → refuse;
  config dir stat failure (permission denied) → refuse; curator
  definition missing or lacking `mode: subagent` → refuse; JSONC
  comments and trailing commas parsed correctly; registration scan
  reports registered/absent and warns on an unparseable scanned
  config without changing the verdict; V1 `plugin` array, V2
  `plugins` array, object entries with `package`, and `-` disable
  markers all resolved.

**Checkpoint** (exact ci.yml Build/Vet/Test steps):
`go build -v ./... && go vet ./... && go test -race -v -count=1
-coverprofile=coverage.out ./...`

## 3. Command Binding and Human Gates

- [x] 3.1 Edit `.opencode/commands/dewey-distill.md`: add
  `agent: curator` to the YAML frontmatter after `description`; add
  `### 0. Startup Gate (MANDATORY)` before `### 1. Lint` instructing
  the agent to run `dewey gate auto-answer` — the only shell command
  the command may run — and to stop before any pipeline stage,
  relaying the tool's output verbatim, on anything other than a pass
  (include the tool-rendered PASS/REFUSED output examples near the
  decision point per DE-002); prefix the scope-gate question and the
  review-gate question with `HUMAN_GATE: ` per the design's marker
  grammar; add constraint bullets that the human-gate marker is
  reserved for gate questions and never prefixes informational lines
  (phrase this without beginning a line with the literal token) and
  that guard relaxation is global-only; reword the opening "The
  command uses MCP tools only." sentence to name the startup gate as
  its single documented shell exception; and add a step that treats a
  gate invocation failure (a missing `dewey` binary or a shell error
  producing no verdict) as a refusal.
- [x] 3.2 Mirror the exact edited content into the `"dewey-distill.md"`
  entry of `deweySlashCommands` in `slash_commands.go`, preserving
  that file's Go string concatenation and `<protect>` escaping so the
  map value stays byte-identical to the tracked command file.
- [x] 3.3 Extend the dewey-distill tests in `cli_test.go`: the
  existing byte-identity test now also asserts both the tracked and
  the scaffolded file contain `agent: curator`; add a structural
  marker test asserting exactly one line beginning with `HUMAN_GATE:`
  in the scope-gate section, exactly one in the review-gate section,
  and none in any other section; add an allowlist reference test
  asserting every Dewey MCP tool named in the command's Instructions
  is within {`lint`, `list_pages`, `compile`, `curate`,
  `store_compiled`, `promote`} and that the agent body's allowlist
  statement names exactly those six tools.

**Checkpoint** (exact ci.yml Build/Vet/Test steps):
`go build -v ./... && go vet ./... && go test -race -v -count=1
-coverprofile=coverage.out ./...`

## 4. Documentation

- [x] 4.1 [P] Update `AGENTS.md`: extend the "Server-side synthesis"
  architecture bullet to state that `/dewey-distill` binds to the
  `curator` agent, runs `dewey gate auto-answer` at startup and
  refuses on a non-pass verdict, and marks both gates with the
  human-gate marker; add a "Recent Changes" bullet for
  `curator-agent-human-gates` describing the user-facing change (the
  curator agent, its least-privilege boundary and MCP allowlist, the
  gate command, and the human-only gates); add a `dewey gate` row to
  the CLI command table; note in the `dewey init` row that it
  scaffolds `.opencode/agents/curator.md` when `.opencode/` exists;
  append `gate` to the `cli.go` subcommand parenthetical in the
  Architecture section; and extend the "Server-side synthesis" bullet
  to name the startup-gate shell invocation as the documented
  exception to its "entirely through MCP tools" statement.
- [x] 4.2 [P] Update `README.md`: extend the `/dewey-distill`
  paragraph (around the current line 739) to describe the curator
  binding, the human-only gates, the startup refusal, and the
  global-only guard relaxation; add the curator agent, its
  permission model (tools booleans, permission carve-out for
  `dewey gate auto-answer`, task/edit/write/patch/webfetch denials,
  question allow), and its MCP allowlist to the documentation; add a
  `### dewey gate` section to the CLI reference with the
  `auto-answer` subcommand, its verdict output, and exit codes;
  extend the `### dewey init` section (around the current line 340)
  and its "This creates:" list (around the current line 303) with
  `.opencode/agents/curator.md` scaffolding.
- [x] 4.3 [P] Reconcile the website documentation with the open
  issue unbound-force/website#371 ("docs: sync dewey distill
  documentation"), which already covers the `/dewey-distill`
  workflow and both human gates: add a comment extending its scope
  with the curator agent, its permission model and MCP allowlist,
  the `dewey gate auto-answer` command, and the published
  `HUMAN_GATE:` marker grammar (uppercase token, single ASCII space,
  start of first line, no leading whitespace, never on informational
  lines). Only file a separate issue if #371 turns out not to cover
  these pages; do not create a duplicate.

**Checkpoint** (exact ci.yml Test step):
`go test -race -v -count=1 -coverprofile=coverage.out ./...`

## 5. Verification

- [x] 5.1 Run the full CI-parity gate, deriving every command from
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
- [ ] 5.2 Perform the manual acceptance matrix and record results:
  the command binding switches the agent from a development session
  and restores it afterward; a session whose `agent: curator` binding
  cannot be resolved (definition removed) is refused by the gate;
  `dewey gate auto-answer` runs under the
  curator (the permission carve-out works) and its PASS and REFUSED
  outputs render as prescribed; `git commit`, `git push`, and an
  arbitrary shell command are refused under the curator; the
  adversarial shell forms `dewey gate auto-answer && <cmd>`,
  `dewey gate auto-answer; <cmd>`, and
  `dewey gate auto-answer $(<cmd>)` are refused under the installed
  host (record the leading `XDG_CONFIG_HOME` assignment result
  rather than asserting it); a subagent
  spawn is refused; an allowlisted MCP tool call (for example
  `lint`) succeeds under the curator and a requested non-allowlisted
  mutation (for example `delete_page`) is refused citing the
  allowlist; the gate verdicts for `curator` listed, `*` listed,
  unreadable config, and no plugin reproduce `dewey gate
  auto-answer`'s unit-tested matrix when configured by hand; both
  gate questions render with the `HUMAN_GATE: ` prefix and an
  auto-answer plugin honoring the external contract leaves them for
  a human.
  **Results (recorded)**: the deterministic verdicts for `curator`
  listed, `*` listed, no plugin config, unreadable/unparseable
  config, JSON-null config, and an unresolved curator definition are
  covered by `gate_test.go`. The host-enforced behaviors — the
  binding switch, shell refusals including the adversarial forms, the
  subagent refusal, and the plugin honoring the marker — require a
  running OpenCode host and were not executable in this environment;
  a human must run them before archive.
- [x] 5.3 Verify constitution alignment (principles I–IV remain
  PASS); confirm `git status --short .opencode/agents/` shows only
  the new `curator.md` with no existing agent tool flag, permission
  rule, mode, or command binding changed; confirm the committed
  OpenCode configuration (`.opencode/opencode.json[c]`, root
  `opencode.json[c]`) contains no `blockRewardHacking: false` or
  `requireStepReferenceInTools: false` and no curator-specific guard
  override (a search for those keys must return nothing); confirm a
  uf scaffold run leaves `.opencode/agents/curator.md` in place.

**Checkpoint** (exact ci.yml Test step):
`go test -race -v -count=1 -coverprofile=coverage.out ./...`

## 6. Review

- [ ] 6.1 Run `/uf.review-council` and resolve findings until all
  reviewers return APPROVE; re-run the Phase 5 checks after any fix.
