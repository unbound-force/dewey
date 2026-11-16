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

  No Phase 1-3 task is [P]-eligible: Phase 1 tasks build
  scaffold.go then scaffold_test.go in dependency order,
  Phase 2 tasks edit scaffold.go then cli.go in dependency
  order, and Phase 3 tasks all edit cli_test.go. Phase 4
  documentation tasks touch disjoint files and are [P].

  Phase checkpoints run the exact Build/Vet/Test steps from
  .github/workflows/ci.yml. The Gaze threshold gate and
  golangci-lint run at the Phase 4 CI-parity gate.
-->

## 0. Preconditions

- [x] 0.1 Confirm the change artifacts under
  `openspec/changes/tool-owned-scaffold-updates/` are committed and
  pushed on `opsx/tool-owned-scaffold-updates` (Spec Commit Gate), and
  that the working tree is clean before implementation starts.

## 1. Scaffold Classification Core (scaffold.go, scaffold_test.go)

- [x] 1.1 Create `scaffold.go` (package main, design D1/D2/D7) with the
  marker primitives: `scaffoldKind` (markdown, yaml);
  `canonicalBody(v string) []byte` normalizing the map value to exactly
  one trailing newline; `scaffoldMarker(body []byte, kind scaffoldKind)
  string` rendering the final-line marker — `<!-- dewey:scaffold
  <version> sha256:<hex> -->` for markdown, `# dewey:scaffold <version>
  sha256:<hex>` for yaml, with `<version>` from the existing `version`
  var and `<hex>` the lowercase SHA-256 of the canonical body;
  `scaffoldPayload(body []byte, kind scaffoldKind) []byte` returning
  canonical body + marker + "\n"; and `parseScaffoldMarker(content
  []byte) (digest string, payload []byte, ok bool)` accepting either
  comment form on the final line per the D1 grammar and returning the
  digest with the exact payload bytes preceding the marker. Add GoDoc
  to every function.
- [x] 1.2 Add `classifyScaffold(existing []byte, exists bool,
  embeddedBody []byte) scaffoldDecision` per design D3 — pure decision
  function with `scaffoldCreate | scaffoldUpdate | scaffoldCurrent |
  scaffoldSkip`: missing file → create; marker digest matches payload
  hash → tool-owned (current on byte-equality with the canonical
  embedded body, else update); no marker and byte-identical to the
  canonical embedded body → update (marker bootstrap); marker absent
  with differing content, malformed digest, or digest mismatch → skip.
  Classification MUST NOT read the version field (D8).
- [x] 1.3 Create `scaffold_test.go` with table-driven unit tests: every
  decision-table row (missing; untouched-current; untouched-drifted;
  marker-less byte-identical; marker-less differing; malformed digest;
  digest mismatch); marker round-trip (`scaffoldPayload` output parses
  back to the same digest and payload); both comment syntaxes;
  canonicalization (value without trailing newline, value already
  ending in one); version-independence (same body, different version
  string → same classification); digest verifiability against
  `crypto/sha256` computed in the test; and YAML resilience — the
  scaffolded form of the real `deweyWorkflowTemplates` entry
  (`scaffoldPayload` with the yaml kind) remains parseable YAML after
  the marker append (parse with `gopkg.in/yaml.v3`), enforcing the
  spec's "remains parseable YAML" scenario (DIO-FR-002).

**Checkpoint (Phase 1)**: `go build ./...`, `go vet ./...`,
`go test -race -count=1 ./...` must pass.

## 2. Shared Writer and Init Rewiring (scaffold.go, cli.go)

- [x] 2.1 Add `scaffoldToolOwnedFiles(dir string, files map[string]string,
  kind scaffoldKind, label string) error` per design D4: sorted key
  iteration; `os.MkdirAll` failure fails init naming the directory;
  read-and-classify per design D3's input-acquisition rules —
  `os.Lstat` first, so a symlinked tool-owned path is skipped as
  customized without reading or writing through it, and an existing
  path whose read fails (permission error, directory at the path,
  transient I/O error) fails init naming the path and is never
  labeled customized; create via
  `os.OpenFile(O_CREATE|O_EXCL|O_WRONLY, 0o644)` with
  remove-on-partial-write and an init error naming the path (the
  existing agent discipline), except that an EEXIST from the
  exclusive open — a concurrent init created the path between
  classification and open — logs `skip (concurrent)` at info naming
  the path and continues without replacing the concurrent writer's
  file; update via `os.CreateTemp` in the target
  directory (chmod 0o644) followed by atomic `os.Rename`, cleaning up
  the temp file and failing init naming the path on any failure so the
  original file is never lost; skip logs `skip (customized)` at info
  with the path and performs no filesystem mutation; create logs
  `scaffolded <label>` and update logs `updated <label>` at info with
  the path; current writes nothing and logs nothing. Factor the
  create arm as a small function so the EEXIST branch is directly
  testable. GoDoc per house
  style; keep `gofmt`/`go vet` clean.
- [x] 2.2 Rewire `newInitCmd` in cli.go: replace the three tool-owned
  scaffold blocks with `scaffoldToolOwnedFiles` calls — slash commands
  and agents behind their existing `.opencode/` existence guards
  (guards and labels unchanged), workflow templates unconditionally
  into `.github/workflows/` — keeping the `.opencode/command/` →
  `.opencode/commands/` migration block ahead of the commands call and
  the trailing "weekly distill workflow template available" hint
  (design D5). Do not touch the user-owned blocks: config.yaml /
  sources.yaml / knowledge-stores.yaml write-once, dcp.jsonc three-way
  logic, `.gitignore` handling, `alreadyInitialized` gating.
- [x] 2.3 Writer unit tests in `scaffold_test.go` over `t.TempDir()`
  trees: create writes the exact `scaffoldPayload`; update replaces
  content and refreshes the marker; current leaves mtime and bytes
  untouched; skip leaves a customized file byte-identical; a failed
  create (read-only directory, skipping as root) removes any partial
  file; a failed update (read-only directory with an existing
  tool-owned file) leaves the original content intact and returns an
  error naming the path. Input-acquisition edges per DIO-FR-004 and
  DI-FR-001: a symlinked tool-owned path (symlink to a marker-valid
  drifted payload) is preserved — the symlink is not replaced by a
  regular file, the referent is not written through, and `skip
  (customized)` is logged; an unreadable existing file (chmod 0o000,
  skipping as root) fails init naming the path rather than logging
  `skip (customized)`; the EEXIST concurrent-create branch (exercised
  by invoking the factored create arm against an already-present
  path) logs `skip (concurrent)`, does not overwrite, and does not
  fail.

**Checkpoint (Phase 2)**: `go build ./...`, `go vet ./...`,
`go test -race -count=1 ./...` must pass.

## 3. Acceptance and Parity Tests (cli_test.go, tracked copies)

- [x] 3.1 Acceptance tests in `cli_test.go` driving `newInitCmd()` with
  `--vault` per the delta specs: (a) re-init refreshes an untouched
  drifted command (fixture = valid marker over a fake older body),
  rewriting it to the embedded payload with a fresh marker; (b) an
  untouched current scaffold is not rewritten; (c) a customized file is
  preserved byte-for-byte; (d) a marker-less file byte-identical to the
  embedded payload gains the marker; (e) a marker-less file differing
  from the embedded payload is preserved (legacy safe default); (f) a
  malformed marker is preserved; (g) untouched agent and workflow
  example are refreshed; (h) first init writes every tool-owned file
  with a verifiable marker; (i) no `.opencode/` → no commands/agents
  scaffolded and `.opencode/` not created, workflow example still
  scaffolded;   (j) migration from `.opencode/command/` still runs and
  the migrated file is then classified (customized fixture preserved);
  (k) outcome observability — capture the logger output over a vault
  with one missing, one untouched-drifted, and one customized command
  and assert exactly one `scaffolded`, one `updated`, and one `skip
  (customized)` event, each naming its path (DIO-FR-006); (l)
  deterministic iteration order — two runs over the same vault state
  produce their outcome events in the same order (DIO-FR-006); (m)
  non-map preservation — a user-authored
  `.opencode/commands/dewey-notes.md` (not a `deweySlashCommands`
  entry) is byte-for-byte untouched (DIO-FR-001; real vaults hold 45
  files from multiple toolchains alongside the nine dewey-* commands,
  so this is a live data-preservation criterion).
- [x] 3.2 Update the byte-parity tests to compare through
  `scaffoldPayload` (design D6): `TestInitCmd_ScaffoldsDeweyPublishFromMap`,
  `TestInitCmd_ScaffoldsDeweyCompileFromMap`,
  `TestInitCmd_ScaffoldsDeweyDistillFromMap`,
  `TestInitCmd_ScaffoldsDistillAutoCommand`,
  `TestInitCmd_ScaffoldsCuratorAgentFromMap`,
  `TestInitCmd_ScaffoldsDistillWorkflowExample`, and
  `TestDistillWorkflowTemplate_ScaffoldParity` — each asserts
  scaffolded file and tracked copy equal the constructed marker+payload
  for the map entry. Content-parsing tests
  (`TestCuratorAgent_AllowlistParity`, `TestDistillerAgent_AllowlistParity`,
  frontmatter shape tests, `TestDeweyDistill_CuratorBindingAndHumanGates`)
  must pass unmodified — the trailing marker line is inert to their
  frontmatter/section parsing.
- [x] 3.3 Regenerate the tracked copies: build the binary and run
  `dewey init --vault .` at the repo root; verify `git status`/`git
  diff` shows only the appended marker line on
  `.opencode/commands/dewey-*.md`, `.opencode/agents/curator.md`,
  `.opencode/agents/distiller.md`, and
  `.github/workflows/dewey-distill.yml.example`, and that `.uf/dewey/`
  config files, `.opencode/dcp.jsonc`, and `.gitignore` are unchanged.
  This exercises the marker-bootstrap path on the repo's own
  byte-identical copies.
- [x] 3.4 Audit every remaining `TestInitCmd_*` test
  (`TestInitCmd_Idempotent`, `TestInitCmd_ReInitScaffoldsNewCommands`,
  `TestInitCmd_MigratesOldCommandDir`,
  `TestInitCmd_MigrationSkipsExistingInNewDir`,
  `TestInitCmd_MigrationKeepsOldDirWithNonDeweyFiles`,
  `TestInitCmd_SkipsExistingSlashCommands`,
  `TestInitCmd_ScaffoldingSkipsExistingDeweyPublish`,
  `TestInitCmd_ScaffoldingSkipsExistingCuratorAgent`,
  `TestInitCmd_CuratorAgentWriteFailureFailsInit`,
  `TestInitCmd_NoOpenCodeDir`, `TestInitCmd_NoAgentsDirWhenNoOpenCodeDir`,
  `TestInitCmd_HasFlags`, `TestInitCmd_DoesNotInstallDistillWorkflow`,
  and the DCP/gitignore tests) — each must pass unmodified (their
  fixtures are marker-less custom or missing files, preserved by the
  decision table per design A2); a failure means the implementation
  drifted, not that a test is stale. No new CLI flag may appear
  (proposal Out of scope).

**Checkpoint (Phase 3)**: `go build ./...`, `go vet ./...`,
`go test -race -count=1 ./...` must pass with the regenerated tracked
copies in place.

## 4. Verification and Documentation

- [ ] 4.1 [P] CI-parity gate derived from `.github/workflows/ci.yml`:
  `go build ./...`, `go vet ./...`, `go test -race -count=1 ./...`, the
  Gaze CRAP threshold step with the repository's flags
  (`gaze crap --format=json --coverprofile=coverage.out --max-crapload=48
  --max-gaze-crapload=45 ./...` after the coverage run), and
  golangci-lint. Record results in the PR body.
- [ ] 4.2 [P] Documentation gate — a README claim-site audit, not just
  the `dewey init` section: grep `never overwrit` and `if absent`
  across README.md and update every claim site the reversed
  invariant touches — the `dewey init` section plus at least
  README.md:315 (setup file listing), :786 (curator-agent section),
  :908 (weekly-distill Enable section), and :911 (the `dewey init`
  comment) — to state ownership-aware refresh (untouched tool-owned
  scaffolds are refreshed on re-init; customized files preserved;
  pre-marker drifted files preserved until deleted and re-init'd).
  Document the stricter write-failure behavior (design R2): init now
  hard-fails on slash-command and workflow-template write failures
  that previously succeeded silently, so operators running init in
  read-only checkouts or CI will see new hard errors naming the
  path. Cover the ownership split (tool-owned vs user-owned,
  including the `.gitignore` append exception), the `dewey:scaffold`
  marker, the `updated` / `skip (customized)` outcomes, and the
  legacy note; AGENTS.md — the `dewey
  init` CLI-table row and the scaffold mention in Key Patterns updated
  to ownership-aware refresh, plus a Recent Changes entry; GoDoc
  covered by 1.1/2.1.
- [ ] 4.3 [P] Website content pipeline per AGENTS.md and the org's
  three-issue convention for operator-facing trust-boundary changes —
  file all three in `unbound-force/website` and record the issue
  numbers here: (1) the docs sync issue (`gh issue create --repo
  unbound-force/website --label docs`, titled "docs: sync dewey init
  scaffold updates documentation") describing the changed init
  behavior, the marker line now present in scaffolded files, the
  stricter write-failure behavior, and the affected `content/docs/`
  pages; (2) a blog issue (label blog) — e.g. "self-describing
  scaffolds: content-addressed ownership markers for safe
  auto-update"; (3) a tutorial issue (label tutorial) — e.g.
  "bringing a drifted vault current after a Dewey upgrade: reading
  the marker, deciding delete-vs-keep".
- [x] 4.4 Constitution alignment verification per the proposal's four
  PASS assessments and the design's Constitution Alignment section —
  confirm composability guards unchanged, no new dependency, outcome
  events with paths emitted, classification pure-function tested — and
  record the confirmation in the PR body.

**Checkpoint (Phase 4)**: re-run `go build ./...`, `go vet ./...`,
`go test -race -count=1 ./...` with the documentation in place.
