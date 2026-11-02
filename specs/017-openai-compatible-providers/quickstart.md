# Quickstart Validation Guide: OpenAI-Compatible Providers

**Feature**: `017-openai-compatible-providers` |
**Date**: 2026-11-02

This guide proves the feature works end to end without a live gateway.
Every command runs from the repository root. Run commands with a clean
environment except for variables the scenario sets explicitly.

## Prerequisites

- Go 1.25 or newer on `PATH`.
- The repository checked out on branch
  `017-openai-compatible-providers`.
- No running Ollama, gateway, or GCP credentials are required.
- Optional only for the live smoke test: network access to a gateway
  and a gateway API key.

## 1. Contract Tests (no external service)

```bash
go test -race -count=1 ./openaicompat ./embed ./llm
```

Expected outcomes:

- The stub-gateway suites pass and prove request paths, headers,
  request bodies, response parsing, retry timing, and probe caching.
- `go test -race` reports no data race between `Available()` and
  request calls.
- No test attempts a network connection.

## 2. Configuration and Factory Tests

```bash
go test -race -count=1 -run 'TestReadEmbeddingConfig|TestReadSynthesisConfig|TestNewEmbedderFromConfig|TestNewSynthesizerFromConfig' ./embed ./llm
```

Expected outcomes:

- `openai-compatible` is accepted by both factories.
- Missing `endpoint` and missing `model` errors name the exact field.
- The unknown-provider error lists `ollama`, `vertex`, and
  `openai-compatible`.
- A gateway embedding configuration with no endpoint does not fall
  back to `OLLAMA_HOST` or localhost.
- A gateway synthesis configuration with no endpoint falls back to
  `DEWEY_SYNTHESIS_ENDPOINT` only.
- Existing `ollama` and `vertex` tests pass unchanged.

## 3. Degradation Smoke Test (no gateway)

This scenario verifies FR-015 and SC-004 without any gateway. Use a
vault configuration that points at a closed local port. `dewey index`
constructs the embedding provider, so the degradation warning is on
this code path; `dewey status` never constructs the provider, which
is why the scenario runs `dewey index` first.

```bash
mkdir -p /tmp/dewey-gateway-smoke/.uf/dewey
cat > /tmp/dewey-gateway-smoke/.uf/dewey/config.yaml <<'YAML'
embedding:
  provider: openai-compatible
  model: stub-model
  endpoint: http://127.0.0.1:1/v1
YAML
go run . index --vault /tmp/dewey-gateway-smoke
go run . status --vault /tmp/dewey-gateway-smoke
```

Expected outcomes:

- `dewey index` completes without exiting on the unreachable gateway.
- The index log names the provider and explains that semantic search
  is unavailable; it does not suggest `ollama pull`.
- `dewey status` still reports index state.

## 4. CI Parity Gate

The commands below are copied from `.github/workflows/ci.yml` and must
all pass before a task is considered complete.

```bash
go mod download
go build -v ./...
go vet ./...
go test -race -v -count=1 -coverprofile=coverage.out ./...
go install github.com/unbound-force/gaze/cmd/gaze@latest
gaze crap --format=json \
  --coverprofile=coverage.out \
  --max-crapload=48 \
  --max-gaze-crapload=45 \
  ./...
```

Expected outcomes:

- Build, vet, and every test package pass.
- The Gaze gate exits zero with CRAP and Gaze CRAP below the
  configured ceilings.
- Contract coverage for the new packages is checked against this
  feature's own local bar of 70 percent — a working target for this
  work item, not a CI gate. The CI gate enforces the CRAP ceilings
  above.

MegaLinter runs golangci-lint, markdownlint, yamllint, and gitleaks in
CI. Locally, run the same linters if installed, and at minimum confirm
`gofmt -l` reports no files for the new packages.

## 5. Credential Safety Check

```bash
rg -n 'DEWEY_EMBEDDING_API_KEY|DEWEY_SYNTHESIS_API_KEY|OPENROUTER_API_KEY' \
  openaicompat embed llm
```

Expected outcomes:

- The variables appear only in key-resolution code and tests.
- No log statement, error format, or generated artifact includes a
  key value.
- Grepping a captured error for a test key returns nothing.

## 6. Optional Live Gateway Smoke Test

Only run this when a gateway and key are available. It is not part of
CI.

```bash
export DEWEY_SYNTHESIS_API_KEY="<key>"
cat > /tmp/dewey-gateway-smoke/.uf/dewey/config.yaml <<'YAML'
synthesis:
  provider: openai-compatible
  model: <gateway-model-identifier>
  endpoint: https://<gateway-host>/v1
YAML
go run . compile --vault /tmp/dewey-gateway-smoke
```

Expected outcomes:

- The request reaches `{endpoint}/chat/completions` with the model
  sent verbatim.
- A successful completion is persisted as a compiled article.
- A wrong key produces a bounded, redacted error suggesting the key
  be verified; the key itself never appears.
- Stopping the gateway produces an actionable unavailable error while
  other commands keep working.

## Success Criteria Mapping

| Criterion | Validated by |
|-----------|--------------|
| SC-001 configuration-only switch | Section 6, plus factory tests |
| SC-002 full embedding coverage | `embed` batch tests: one request per batch, one vector per input, order restored (Section 1) |
| SC-003 Ollama and Vertex unchanged | Existing suites in Section 4 |
| SC-004 degraded tools within one second | Section 3, probe test with timeout |
| SC-005 rate-limit recovery | `openaicompat` retry tests |
| SC-006 zero credential exposure | Section 5 and redaction tests |
| SC-007 actionable missing-field errors | Section 2 |

## Troubleshooting

- `requires endpoint (embedding.endpoint)`: set the endpoint in
  `config.yaml` or `DEWEY_EMBEDDING_ENDPOINT`. `OLLAMA_HOST` is not a
  gateway fallback.
- `requires model (embedding.model)`: set the model identifier
  exactly as the gateway expects it.
- Authentication error: set the provider-scoped key variable. The
  shared `DEWEY_API_KEY` also works. No key is needed for an
  unauthenticated self-hosted server.
- 429 errors that persist: confirm the retry budget was exhausted
  (initial attempt plus five retries). Check the gateway's
  `Retry-After` behavior.
