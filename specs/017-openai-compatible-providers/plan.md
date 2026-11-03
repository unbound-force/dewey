# Implementation Plan: OpenAI-Compatible API Providers

**Branch**: `017-openai-compatible-providers` | **Date**: 2026-11-02 |
**Spec**: [spec.md](./spec.md)

**Input**: Feature specification from
`/specs/017-openai-compatible-providers/spec.md`

## Summary

Add an `openai-compatible` embedding provider and an
`openai-compatible` synthesis provider that speak the OpenAI v1 wire
protocol over plain `net/http`. This lets Dewey use hosted or
self-hosted gateways (ENMaas, OpenRouter, vLLM, LiteLLM) for
`POST /embeddings`, `POST /chat/completions`, and the `GET /models`
health probe. Provider selection stays configuration-only, credentials
stay environment-only, and Ollama and Vertex behavior is unchanged.
Retry, availability caching, and graceful degradation follow the
Vertex provider pattern.

A new leaf package, `openaicompat`, owns the security-sensitive
transport surface exactly once: API-key resolution, base-URL
normalization, authenticated JSON requests, 429 retry with backoff,
credential-free error excerpts, and the cached availability probe.
`embed` and `llm` keep their own `ProviderConfig`, request/response
types, parsing, and factory functions, preserving the 016 decision to
decouple provider configuration.

## Technical Context

**Language/Version**: Go 1.25 (per `go.mod`)

**Primary Dependencies**: stdlib `net/http`, `encoding/json`,
`context`, `net/url`, `math/rand/v2`, `sync`, `time`; existing
`github.com/charmbracelet/log`. No new module dependency and no vendor
SDK (FR-018).

**Storage**: N/A — no schema, table, or file-format change. Embeddings
continue to persist through `store.Store` as `float32` vectors.

**Testing**: stdlib `testing` with `net/http/httptest` stub gateways,
`t.Setenv`, and `t.TempDir`. No live gateway is required
(Constitution IV).

**Target Platform**: macOS and Linux (darwin/linux, amd64/arm64 per
the release matrix).

**Project Type**: Single Go module — MCP server plus CLI
(`main.go`, `cli.go`, `server.go`).

**Performance Goals**: the availability probe returns within one
second on failure (SC-004); a batch of N texts uses one embedding
request (US2-2); 429 retries stop at the initial attempt plus at most
five retries (FR-012).

**Constraints**: pure Go, no CGO (FR-018); API keys never appear in
`config.yaml`, logs, errors, or generated artifacts (FR-006, FR-019);
no remote contact unless the user selects `openai-compatible`
(Constitution, Local-Only Processing); response bodies read with the
same 50 MiB limit the Vertex providers use.

**Scale/Scope**: vault-scale indexing (thousands of chunks per run)
through one embedder instance; two additional factory cases; no new
MCP tool or CLI command.

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1
design.*

| Principle | Gate | Verdict | Evidence |
|-----------|------|---------|----------|
| I. Composability First | Dewey remains usable without any remote provider; Ollama stays the default; keyword and graph tools never depend on a gateway | PASS | Provider is opt-in through `config.yaml`; the existing `Available()` degradation path is reused; `openai-compatible` is one factory case among three |
| II. Autonomous Collaboration | No new runtime coupling; no MCP tool contract change | PASS | No MCP tool is added or changed; the structured responses of `compile` and `curate` gain an `unavailable_reason` field on the degradation path while their success shapes stay unchanged; `Embedder` and `Synthesizer` interfaces are unchanged |
| III. Observable Quality | Provider state stays auditable; failures name the exact field or environment variable | PASS | Cached `/models` probe, actionable unavailable reason, provider-aware doctor and startup logs, and zero credential material |
| IV. Testability | Every package testable in isolation; contract behavior covered; CRAP and coverage ratchets respected | PASS | `httptest` stub gateways only; table-driven contract tests; small functions keep CRAP under the CI gate |
| Local-Only Processing | No remote contact unless explicitly configured | PASS | A gateway client is built only for `provider: openai-compatible`; there is no default endpoint and no discovery |
| Upstream Stewardship | Inherited MCP tool contracts are unchanged | PASS | Existing provider suites run unchanged (US4); no tool input or output changes |
| Development Standards | Pure Go, no CGO, minimal dependencies | PASS | Stdlib-only transport; no new module required |

No violations. Complexity Tracking is not required.

**Post-Design Re-Check (after Phase 1)**: PASS — the data model,
contracts, and quickstart add no dependency, interface change, or
remote default. See `data-model.md` and `contracts/`.

## Project Structure

### Documentation (this feature)

```text
specs/017-openai-compatible-providers/
├── plan.md              # This file (/speckit.plan output)
├── research.md          # Phase 0 decisions and alternatives
├── data-model.md        # Phase 1 entities and validation rules
├── quickstart.md        # Phase 1 runnable validation guide
├── contracts/
│   ├── openai-compatible-wire.md
│   └── configuration.md
├── checklists/
│   └── requirements.md
└── tasks.md             # Phase 2 output (/speckit.tasks)
```

### Source Code (repository root)

```text
openaicompat/                  # NEW leaf package (stdlib + log only)
├── client.go                  # Client, NewClient, PostJSON, probe
├── auth.go                    # FR-007 key precedence (never logged)
├── endpoint.go                # FR-010 normalization and path join
├── retry.go                   # FR-011/012/013 429 retry and backoff
├── errors.go                  # FR-020 bounded, redacted excerpt
├── availability.go            # FR-014 /models probe and 30s cache
└── *_test.go                  # stub-gateway contract tests

embed/
├── provider.go                # add factory case and supported list
├── config.go                  # gateway endpoint has no Ollama fallback
├── openai_compatible.go       # NEW: OpenAICompatibleEmbedder
├── openai_compatible_test.go  # NEW
└── embed.go                   # add UnavailableReasoner capability

llm/
├── provider.go                # add factory case and supported list
├── config.go                  # synthesis env fallback for gateway
├── openai_compatible.go       # NEW: OpenAICompatibleSynthesizer
├── openai_compatible_test.go  # NEW
└── llm.go                     # add UnavailableReasoner capability

cli.go                         # doctor branches for the new provider
tools/semantic.go              # actionable unavailable error (FR-015)
tools/compile.go               # actionable unavailable error (FR-015)
tools/curate.go                # actionable unavailable error (FR-015)
main.go                        # provider-specific startup logging
```

**Structure Decision**: The repository uses a flat package layout.
`openaicompat` is a new leaf package depending only on the standard
library and `charmbracelet/log`; `embed` and `llm` both import it.
Dependency direction is `embed → openaicompat`,
`llm → openaicompat`, and the existing `llm → embed` constant import.
No cycle is introduced. Note: this extends an existing divergence
from the 013 llm-interface leaf-dependency invariant (`llm` MUST NOT
import Dewey packages): `llm/provider.go` and `llm/config.go`
already import `embed` (since spec 016's provider-config
consolidation), and 017 adds `llm → openaicompat`. The divergence is
recorded in `research.md` R13; `openaicompat` itself stays a true
leaf (stdlib plus `charmbracelet/log` only). `embed` and `llm` keep
separate configuration, request/response types, and factories, so the
intentional configuration decoupling from spec 016 is preserved. The shared
package exists because a third gateway protocol has now emerged and
the FR-007 credential chain is security-sensitive: two copies would
drift. See `research.md` R1 for the alternatives considered.

## Key Design Decisions

### D1: One transport package, two thin providers

`openaicompat` implements the transport policy once. Each provider
implements only its request body, response shape, and parsing. This
keeps the credential chain single-sourced and avoids roughly 150
duplicated lines. Configuration, constructor validation, and factories
stay per package.

### D2: Credentials are environment-only

No `api_key` field is added to `ProviderConfig`. The parsed `configFile`
structs carry a warn-only `api_key` field so a key present in
`config.yaml` can be detected and warned about at startup and in `dewey
doctor` (FR-024); the value is never copied into `ProviderConfig` and
never authenticates a request. The factory resolves the key at construction
time using the FR-007 chain: provider-scoped variable, then
`DEWEY_API_KEY`, then `OPENROUTER_API_KEY` when the endpoint host is
`openrouter.ai`. Empty variables count as unset. The `Authorization`
header is sent only when a key resolves, so unauthenticated
self-hosted servers work. Keys are never logged and are redacted from
error excerpts.

### D3: Availability is a cached `/models` probe

`Available()` performs one authenticated `GET {base}/models` with a
one-second timeout, caches the result for thirty seconds, and is safe
for concurrent use. Any 2xx means available. Connection errors,
401, 403, and other statuses mean unavailable; the probe records a
credential-free reason. Model membership is not checked, because
gateway aliases, model rewriting, and lazy loading make it
unreliable. A 401 or 403 produces the FR-008 authentication message,
which is surfaced through an optional capability interface rather than
a change to `Embedder` or `Synthesizer`.

### D4: Configuration precedence is unchanged per code path

For `openai-compatible`, embedding remains env-first
(`DEWEY_EMBEDDING_ENDPOINT` over `config.yaml`) and synthesis remains
config-first (`config.yaml` over `DEWEY_SYNTHESIS_ENDPOINT`). The
Ollama-only fallbacks (`OLLAMA_HOST` and the localhost default) do not
apply to gateway endpoints. A missing gateway endpoint is a factory
error naming `embedding.endpoint` or `synthesis.endpoint` (FR-009,
FR-022). Ollama and Vertex resolution is untouched (FR-017).

### D5: Retry is bounded and cancellation-aware

On HTTP 429, an integer-seconds `Retry-After` is honored with a
sixty-second cap; otherwise exponential backoff starts at one second,
doubles per attempt, adds plus or minus 25 percent jitter, and caps at
sixty seconds. The loop runs the initial request plus at most five
retries. A canceled context returns immediately with the context error
(FR-011, FR-012, FR-013).

### D6: Response parsing is strict and order-preserving

The embedder sends one request for the whole batch and returns one
vector per input. Response entries are reordered by their `index`
field; a count mismatch, duplicate index, or out-of-range index is a
descriptive error. The synthesizer reads the first choice's assistant
content and errors on empty or missing content rather than persisting
an empty article. Requests are non-streaming and never use tool
calling, function calling, or structured output.

### D7: Errors are bounded, redacted, and provider-named

Non-success responses produce `openai-compatible <capability> error
(HTTP <status>): <excerpt>`, where the excerpt is capped at 512 bytes
and has any occurrence of the resolved key replaced with
`[REDACTED]`. Parse errors are prefixed the same way. The HTTP client
installs an explicit redirect policy that strips `Authorization` unless
the destination matches the initial request's host (port-insensitive,
case-insensitive) and scheme; subdomains and same-host scheme
downgrades are treated as different origins.

### D8: Diagnostics become provider-aware

`dewey doctor` gains an `openai-compatible` branch for the embedding
and synthesis layers that checks configuration completeness and probe
status without printing credentials. The doctor's unknown-provider
message lists all three supported values. Startup logging, `dewey
doctor`, and the shared indexing helper used by `dewey index`,
`reindex`, `compile`, `curate`, and `lint` stop suggesting
`ollama pull` for a gateway provider while keeping the Ollama path
byte-for-byte unchanged.

### D9: Quality gates come from CI, not memory

Local verification replicates `.github/workflows/ci.yml` exactly:
`go build -v ./...`, `go vet ./...`,
`go test -race -v -count=1 -coverprofile=coverage.out ./...`, then
`gaze crap --format=json --coverprofile=coverage.out
--max-crapload=48 --max-gaze-crapload=45 ./...`. MegaLinter
(golangci-lint, markdownlint, yamllint, gitleaks) is the lint gate.

## Testing & Quality Strategy

**Contract coverage** targets observable behavior: request path,
method, headers, model string, input count and order, retry timing,
404/401/429 handling, probe caching, and degraded tool responses.
Tests never assert private struct layouts or retry constants by name.

- `openaicompat`: endpoint normalization and join, key precedence,
  request shape, 429 then success, retry exhaustion, context
  cancellation, redaction and truncation, probe outcomes, cache
  behavior, concurrent `Available` plus request, and redirect
  authorization stripping (cross host, subdomain, scheme downgrade).
- `embed`: `/embeddings` path, one request per batch, count mismatch,
  out-of-order index reorder, empty input without a network call,
  model identifiers with slashes verbatim, and non-ASCII round-trip.
- `llm`: `/chat/completions` path, user message shape, first-choice
  extraction, empty-content error, and non-2xx error text.
- Factories and config: registration, supported-provider list,
  missing field errors naming the exact key, gateway endpoint with no
  Ollama fallback, and unchanged Ollama/Vertex suites.
- Tools and CLI: actionable unavailable message, doctor branch, and
  provider-aware startup logging.

**CRAP management**: split request building, response parsing, retry
delay, excerpt redaction, and probe evaluation into small functions
with table-driven tests so each stays well below the CI CRAP gate.

## Risks & Mitigations

- **Key leakage through error bodies**: gateways may echo request
  material. Mitigation: redact the resolved key from every excerpt and
  never include request headers or bodies in errors or logs.
- **False unavailability on slow gateways**: a one-second probe can
  mark a healthy but slow `/models` route unavailable for the cache
  window. Mitigation: accepted trade-off — while the cache reports
  unavailable, semantic tools degrade to keyword-only (FR-015) and the
  index path skips embedding generation rather than attempting
  requests; the gateway is reported available again once the cache
  expires after thirty seconds and a fresh probe succeeds. A remote
  gateway whose `/models` route consistently exceeds one second is
  misconfigured for this probe; SC-004's one-second actionable-error
  requirement takes precedence. Documented in `research.md` R3.
- **Divergent behavior between embedding and synthesis**: mitigated by
  one shared transport implementation and one key-resolution chain.
- **Accidental behavior change for Ollama or Vertex**: mitigation:
  branch only on `openai-compatible`, keep the existing fallback for
  other providers, and run existing suites unchanged.
- **Pre-existing gap observed, out of scope**: `dewey doctor`'s
  embedding layer treats Vertex as Ollama. This plan does not change
  Vertex diagnostics (FR-017); record it as a follow-up issue rather
  than expanding scope.
- **Oversized batches rejected by a gateway**: Dewey surfaces the
  gateway error and never drops inputs; automatic splitting is out of
  scope.

## Complexity Tracking

No constitution violations. No entries required.
