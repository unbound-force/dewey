<!--
  [P] marks tasks eligible for parallel execution.
  Add [P] when a task: (a) touches different files from
  other [P] tasks in the same group, (b) has no dependency
  on prior tasks in the group, (c) can safely execute
  without ordering constraints.
  Do NOT add [P] when tasks modify the same file — parallel
  workers will cause merge conflicts.
  Tasks without [P] run sequentially first, then [P]
  tasks run in parallel.

  All production work lands in cli.go and all tests in
  cli_test.go, so the code tasks are sequential. The
  website-sync task touches no repo file and is
  parallel-eligible once the behavior is implemented.
-->

## 1. Implementation (cli.go)

- [ ] 1.1 Add `queryEmbeddingStatus(deweyDir string) (model string, available bool)` to `cli.go` next to the other status helpers. Resolve the model with `embed.ReadEmbeddingConfig(deweyDir)`; construct the embedder with `embed.NewEmbedderFromConfig(cfg)`; return `embedder.Available()`; on construction error return the resolved model and `false`. Diagnostic-only: no `ensureOllama` call, no subprocess spawn (design D1-D3, STA-FR-001/STA-FR-002).
- [ ] 1.2 Wire it into `newStatusCmd`'s `RunE`: assign `data.EmbeddingModel, data.EmbeddingAvailable = queryEmbeddingStatus(deweyDir)` alongside the existing `queryTrustStatus` call, remove the `EmbeddingModel: readEmbeddingModel(deweyDir)` initialization from `queryStoreStatus`, and delete `readEmbeddingModel` entirely (verify with a repo-wide search that no references remain).

## 2. Tests (cli_test.go)

- [ ] 2.1 Add a status test-vault helper modeled on `newGatewayDoctorVault` (temp vault, `config.yaml` contents as parameter, `t.Setenv` clearing `XDG_CONFIG_HOME` to a temp dir, `DEWEY_EMBEDDING_MODEL`, `DEWEY_EMBEDDING_ENDPOINT`, and `OLLAMA_HOST`), then cover the STA-FR-001 model-resolution scenarios: scaffolded comment stripped to `granite-embedding:30m`, `DEWEY_EMBEDDING_MODEL` override, no-config default, global-config fallback. Assert exact `embeddingModel` values (TC-009).
- [ ] 2.2 Cover the STA-FR-002 availability scenarios against `queryEmbeddingStatus` / `dewey status --json`: an `httptest` server serving `/api/tags` with the configured model yields `true`; a closed endpoint yields `false` with the command still exiting 0; a tags list without the model yields `false`; a Vertex config missing `project` yields `false` without command failure. Pin every endpoint through the vault config or `DEWEY_EMBEDDING_ENDPOINT` — no test may depend on a host Ollama (constitution IV).
- [ ] 2.3 Assert the STA-FR-003 JSON key set is unchanged (all pre-existing keys present, no additions) and that the text output's `Model:` line shows the clean model with no comment text. Verify the existing `TestStatusCmd_*` suite still passes unmodified.

## 3. Verification and sync

- [ ] 3.1 [P] Run the CI parity gate locally with the exact commands from `.github/workflows/ci.yml`: `go build -v ./...`, `go vet ./...`, `go test -race -v -count=1 -coverprofile=coverage.out ./...`, then `gaze crap --format=json --coverprofile=coverage.out --max-crapload=48 --max-gaze-crapload=45 ./...`. All must pass before the change is declared complete (AGENTS.md CI Parity Gate).
- [ ] 3.2 [P] File the website documentation-sync issue in `unbound-force/website` (`gh issue create --repo unbound-force/website --label docs`, title `docs: sync dewey status documentation`): `dewey status --json` now reports a clean `embeddingModel` (serve-resolved, honoring `DEWEY_EMBEDDING_MODEL` and the global config) and a real `embeddingAvailable` probe instead of a constant `false`; identify any `content/docs/` pages showing status output.
- [ ] 3.3 Verify constitution alignment for the implemented change: hermetic tests only (no external services — IV), `dewey status` still fully functional with no embedding provider configured (II), JSON keys and provenance of reported values documented (III), no new runtime coupling or communication channels (I). Record the result in the PR body.
