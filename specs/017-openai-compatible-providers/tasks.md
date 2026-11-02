---

description: "Task list for OpenAI-Compatible API Providers"
---

# Tasks: OpenAI-Compatible API Providers

**Input**: Design documents from
`/specs/017-openai-compatible-providers/`

**Prerequisites**: plan.md (required), spec.md (required for user
stories), research.md, data-model.md, contracts/, quickstart.md

**Tests**: The feature spec explicitly puts "automated tests that use
stub gateways and need no live service" in scope, and plan.md defines
a testing and quality strategy. Test tasks are therefore included and
must fail before their implementation task is considered complete.

**Organization**: Tasks are grouped by user story so each story is
independently implementable, testable, and demonstrable.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependency on an
  incomplete task in the same group)
- **[Story]**: Which user story this task belongs to (US1, US2, US3,
  US4)
- Include exact file paths in descriptions

## Path Conventions

This repository is a single Go module with a flat package layout.
Paths below are relative to the repository root.

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Establish the new package surface and a clean baseline.

- [ ] T001 Create `openaicompat/doc.go` with package GoDoc describing the shared OpenAI-compatible transport, the import direction (`embed → openaicompat`, `llm → openaicompat`), and the rule that `openaicompat` imports only the standard library and `charmbracelet/log`
- [ ] T002 [P] Capture the pre-change baseline by running `go build -v ./...` and `go test -race -count=1 ./embed ./llm ./tools`; confirm `embed/provider_test.go`, `embed/vertex_test.go`, `llm/provider_test.go`, and `llm/vertex_test.go` are green before any edit

**Checkpoint**: The package surface exists and the existing provider
suites are known green.

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Build the single shared transport policy every story
depends on. No user story work can begin until this phase is
complete.

**⚠️ CRITICAL**: All four user stories depend on this phase.

- [ ] T003 [P] Implement base-URL normalization and operation-path join in `openaicompat/endpoint.go`: prepend `http://` when no scheme is present, remove trailing slashes, and append exactly one of `/embeddings`, `/chat/completions`, or `/models` (FR-010, R4)
- [ ] T004 [P] Implement capability-scoped API-key resolution in `openaicompat/auth.go`: provider-scoped variable, then `DEWEY_API_KEY`, then `OPENROUTER_API_KEY` only when the endpoint host is exactly `openrouter.ai`; an empty value counts as unset; no configuration-file key is read (FR-006, FR-007, R2)
- [ ] T005 [P] Implement retry delay computation in `openaicompat/retry.go`: integer-seconds `Retry-After` capped at 60 seconds, otherwise exponential backoff from 1 second, doubling per attempt, plus or minus 25 percent jitter, capped at 60 seconds; expose the initial-attempt-plus-five-retries budget and a context-aware wait (FR-011, FR-012, FR-013, R7)
- [ ] T006 [P] Implement bounded, credential-free error excerpts in `openaicompat/errors.go`: format `openai-compatible <capability> error (HTTP <status>): <excerpt>`, replace every occurrence of the resolved key with `[REDACTED]`, then cap the excerpt at 512 bytes with a truncation marker — redaction precedes truncation so a key straddling the boundary cannot leak a prefix (FR-019, FR-020, R8)
- [ ] T007 Implement `openaicompat/client.go`: `Client` with normalized base URL, resolved key, capability-scoped `*http.Client` timeout (30 seconds embedding, 300 seconds synthesis), and availability-cache fields; `NewClient`; `PostJSON` that sets `Content-Type`, sets `Authorization` only when a key resolved, reads bodies with the existing 50 MiB limit, runs the 429 retry loop, and relies on `net/http` default cross-host redirect behavior (FR-018, FR-025, R5, R8)
- [ ] T008 Implement the availability probe and 30-second cache in `openaicompat/availability.go`: authenticated `GET {base}/models` with a one-second timeout, any 2xx means available, connection errors and 401/403 mean unavailable with a credential-free reason, double-checked locking prevents duplicate probes, and the probe response body is drained, bounded, and closed even though only the status is used (FR-014, FR-025, R3, D3)
- [ ] T009 [P] Write `openaicompat/endpoint_test.go` contract tests for scheme normalization, trailing-slash removal, exactly-one path append, no double append when the endpoint already ends in an operation path, and rejection with a field-naming configuration error for adversarial endpoints: empty or whitespace-only, non-HTTP schemes such as `file://` and `ftp://`, query strings or fragments, and embedded userinfo (FR-010, wire contract Base URL validation)
- [ ] T010 [P] Write `openaicompat/auth_test.go` tests for precedence order, capability scoping, empty-as-unset, exact-host `openrouter.ai` gating with the port stripped and the host compared case-insensitively (no subdomain match), and no key resolving to an empty string (FR-007)
- [ ] T011 [P] Write `openaicompat/retry_test.go` tests for honored and capped `Retry-After`, malformed `Retry-After` values (negative, zero, non-numeric, and HTTP-date) treated as absent, exponential growth with jitter bounds, exhaustion after the initial attempt plus five retries, and prompt context cancellation during a wait (FR-011, FR-012, FR-013)
- [ ] T012 [P] Write `openaicompat/errors_test.go` tests for the 512-byte cap, truncation marker, full key redaction including repeated occurrences, redaction of a key straddling the 512-byte truncation boundary (redact before truncating), all-C0-control-character and ESC-sequence stripping from excerpts, and absence of request headers, bodies, or URLs from error text (FR-019, FR-020)
- [ ] T013 [P] Write `openaicompat/client_test.go` stub-gateway tests for the request method and path, `Content-Type`, `Authorization` presence and absence, non-2xx error text, malformed-JSON parse error naming the provider, 429-then-success, capability-scoped client timeouts (30 seconds embedding, 300 seconds synthesis, FR-025), and `Authorization` stripping on a cross-host redirect (FR-018, FR-020, FR-025, R8)
- [ ] T014 [P] Write `openaicompat/availability_test.go` tests for 2xx available, connection-error/401/403 unavailable reasons, the 401/403-with-no-key reason naming the expected environment variables (SC-007), the 401/403-with-key "verify the key" probe reason (SC-007), the 30-second cache returning without a second network call, and concurrent `Available()` plus request calls under `-race` (FR-014, US3 edge case)

**Checkpoint**: Run `go build -v ./...` and
`go test -race -count=1 ./openaicompat`. Do not begin a user story
until both pass.

---

## Phase 3: User Story 1 - OpenAI-compatible synthesis (Priority: P1) 🎯 MVP

**Goal**: Let `dewey compile` and curation synthesize through an
OpenAI-compatible gateway selected in `config.yaml`.

**Independent Test**: Point a stub gateway at the configured endpoint,
run `dewey compile` with stored learnings, and verify the request path,
the verbatim model field, and that the returned assistant text is
persisted as the compiled article.

### Tests for User Story 1

Contract tests are written first and must fail before the
implementation tasks below are considered complete.

- [ ] T019 [P] [US1] Write `llm/openai_compatible_test.go` stub-gateway tests for the `/chat/completions` path, verbatim slash-and-hyphen models, a single user message, `stream: false`, absence of `max_tokens`/tools/response formats, first-choice extraction, empty-content error, 401 bounded redacted error naming `DEWEY_SYNTHESIS_API_KEY` (SC-007), and non-ASCII round-trip (FR-004, FR-005, FR-021, US1)
- [ ] T020 [P] [US1] Extend `llm/provider_test.go` with gateway factory and configuration cases: missing endpoint names `synthesis.endpoint`, missing model names `synthesis.model`, factory construction succeeds with no API key in the environment when the endpoint is present (FR-008 optional key), unknown provider lists all three values, config-first precedence with `DEWEY_SYNTHESIS_ENDPOINT`, no `OLLAMA_HOST` fallback, and unchanged `ollama`/`vertex` behavior (FR-008, FR-009, FR-016, FR-022)

### Implementation for User Story 1

- [ ] T015 [P] [US1] Add gateway synthesis endpoint resolution in `llm/config.go`: per-vault then global `synthesis.endpoint`, then `DEWEY_SYNTHESIS_ENDPOINT`; do not consult `OLLAMA_HOST` or the localhost default for `openai-compatible` (FR-022, D4)
- [ ] T016 [P] [US1] Add the optional `UnavailableReasoner` capability interface (`UnavailableReason() string`) in `llm/llm.go` without changing the `Synthesizer` interface (FR-008, FR-014, R3)
- [ ] T017 [US1] Implement `llm/openai_compatible.go`: `OpenAICompatibleSynthesizer` validates `synthesis.model` and `synthesis.endpoint`, sends `{"model": <verbatim>, "messages": [{"role": "user", "content": <prompt>}], "stream": false}` to `/chat/completions`, extracts `choices[0].message.content`, errors on empty or missing content, and implements `Available`, `ModelID`, and `UnavailableReason` (FR-004, FR-005, FR-021)
- [ ] T018 [US1] Register the `openai-compatible` case in `llm/provider.go` and include it in the unknown-provider supported list as `ollama, vertex, openai-compatible` (FR-016)

**Checkpoint**: Run `go vet ./llm` and
`go test -race -count=1 ./llm`. User Story 1 is independently
demonstrable once both pass.

---

## Phase 4: User Story 2 - OpenAI-compatible embeddings (Priority: P2)

**Goal**: Power indexing and semantic search with an
OpenAI-compatible embedding gateway when Ollama is unavailable.

**Independent Test**: Point a stub gateway at the configured endpoint,
run `dewey index`, and verify the request path and that one vector per
input is stored in input order.

### Tests for User Story 2

Contract tests are written first and must fail before the
implementation tasks below are considered complete.

- [ ] T025 [P] [US2] Write `embed/openai_compatible_test.go` stub-gateway tests for the `/embeddings` path, one request per batch, N inputs to N ordered vectors, out-of-order index reordering, count-mismatch error, duplicate and out-of-range index errors, empty input with no network call, verbatim slash-and-hyphen models, non-ASCII round-trip, and 401 redacted error naming `DEWEY_EMBEDDING_API_KEY` (SC-007) (FR-003, FR-023, US2)
- [ ] T026 [P] [US2] Extend `embed/provider_test.go` with gateway factory and configuration cases: missing endpoint names `embedding.endpoint`, missing model names `embedding.model`, factory construction succeeds with no API key in the environment when the endpoint is present (FR-008 optional key), unknown provider lists all three values, env-first precedence, no `OLLAMA_HOST` fallback, and unchanged `ollama`/`vertex` behavior (FR-008, FR-009, FR-016, FR-022)

### Implementation for User Story 2

- [ ] T021 [P] [US2] Add gateway embedding endpoint resolution in `embed/config.go`: `DEWEY_EMBEDDING_ENDPOINT`, then per-vault, then global `embedding.endpoint`; do not consult `OLLAMA_HOST` or the localhost default for `openai-compatible` (FR-022, D4)
- [ ] T022 [P] [US2] Add the optional `UnavailableReasoner` capability interface (`UnavailableReason() string`) in `embed/embed.go` without changing the `Embedder` interface (FR-008, FR-014, R3)
- [ ] T023 [US2] Implement `embed/openai_compatible.go`: `OpenAICompatibleEmbedder` validates `embedding.model` and `embedding.endpoint`, returns `nil, nil` for an empty batch without a network call, sends one `{"model": <verbatim>, "input": [...]}` request to `/embeddings`, reorders `data` by `index`, converts to `float32`, and errors on count mismatch, duplicate index, or out-of-range index; implement `Embed`, `Available`, `ModelID`, and `UnavailableReason` (FR-003, FR-023, US2)
- [ ] T024 [US2] Register the `openai-compatible` case in `embed/provider.go` and include it in the unknown-provider supported list as `ollama, vertex, openai-compatible` (FR-016)

**Checkpoint**: Run `go vet ./embed` and
`go test -race -count=1 ./embed`. User Story 2 is independently
demonstrable once both pass.

---

## Phase 5: User Story 3 - Resilience and graceful degradation (Priority: P2)

**Goal**: Retry rate-limited gateway calls and keep non-semantic tools
working while reporting semantic features as unavailable.

**Independent Test**: Use a stub gateway that returns 429 twice and
then succeeds with `Retry-After`, then use an unreachable endpoint and
verify non-semantic MCP tools still respond.

### Tests for User Story 3

Contract tests are written first and must fail before the
implementation tasks below are considered complete.

- [ ] T032 [P] [US3] Extend `tools/semantic_test.go` with a degraded-provider test asserting the unavailable error carries the provider reason and remains actionable (FR-015, SC-004)
- [ ] T033 [P] [US3] Extend `tools/compile_test.go` with a prompt-mode test asserting the synthesis unavailable reason is reported without losing the prompts (FR-015)
- [ ] T034 [P] [US3] Extend `tools/curate_test.go` with a prompt-mode test asserting the synthesis unavailable reason is reported without losing the extraction prompts (FR-015)
- [ ] T035 [P] [US3] Extend `cli_test.go` with doctor tests for a complete `openai-compatible` embedding and synthesis configuration, an incomplete configuration naming the missing field, the updated unknown-provider message, the cleartext-credential warning for an `http://` non-loopback endpoint with a resolved key, the ignored-`api_key` warning when the field appears in `config.yaml`, and the shared indexing helper's unavailability warning naming the provider without an `ollama pull` hint (FR-024, D8, R9)
- [ ] T036 [P] [US3] Extend `main_test.go` with a startup test proving an unreachable `openai-compatible` endpoint degrades to keyword mode, logs the provider reason, does not suggest `ollama pull`, and emits the cleartext-credential warning when a key would transit `http://` to a non-loopback host (FR-015, FR-024, SC-004)

### Implementation for User Story 3

- [ ] T027 [P] [US3] Make the semantic MCP tools in `tools/semantic.go` surface the provider's unavailable reason through `UnavailableReasoner` while keeping an actionable fallback message and removing Ollama-only wording (FR-015, SC-004)
- [ ] T028 [P] [US3] Surface the synthesis unavailable reason in the prompt-mode response in `tools/compile.go` while preserving prompt mode as the degradation path (FR-015, configuration contract)
- [ ] T029 [P] [US3] Surface the synthesis unavailable reason in the prompt-mode response in `tools/curate.go` while preserving prompt mode as the degradation path (FR-015, configuration contract)
- [ ] T030 [P] [US3] Make `dewey doctor` provider-aware in `cli.go`: add `openai-compatible` branches for the embedding and synthesis layers that check configuration completeness and probe status without secrets, list `openai-compatible` in both unknown-provider messages, warn when a resolved key would transit `http://` to a non-loopback host or an `api_key` field appears in `config.yaml`, and update the shared `createIndexEmbedder` unavailability warning — reached by `dewey index`, `reindex`, `compile`, `curate`, and `lint` — so the `ollama pull` hint is emitted only for Ollama (FR-015, FR-024, D8, R9)
- [ ] T031 [P] [US3] Make startup logging provider-aware in `main.go`: for `openai-compatible`, log the provider, endpoint, and unavailable reason, do not emit an `ollama pull` hint, and warn when a resolved key would transit `http://` to a non-loopback host or an `api_key` field appears in `config.yaml`; leave the Ollama branch byte-for-byte unchanged (FR-015, FR-024, D8, R9)

**Checkpoint**: Run `go test -race -count=1 ./tools .` and the
`openaicompat` retry tests. User Story 3 is independently
demonstrable once both pass.

---

## Phase 6: User Story 4 - Compatibility and separate gateways (Priority: P3)

**Goal**: Preserve existing `ollama` and `vertex` behavior and support
two gateways with two credentials in one process.

**Independent Test**: Run the existing provider suites unchanged, then
configure embedding and synthesis with different endpoints and keys
and verify each request authenticates correctly.

### Tests for User Story 4

- [ ] T037 [US4] Add `openaicompat/isolation_test.go` (package `openaicompat_test`) that builds an embedding provider and a synthesis provider against two `httptest` servers with distinct `DEWEY_EMBEDDING_API_KEY` and `DEWEY_SYNTHESIS_API_KEY` values, then asserts each server receives its own `Bearer` key and never the other's (FR-007, US4-3)
- [ ] T038 [US4] Verify backward compatibility by running `embed/vertex_test.go`, `llm/vertex_test.go`, `embed/provider_test.go`, and `llm/provider_test.go` unchanged and confirming an omitted `provider` still constructs Ollama and an existing `vertex` section still constructs Vertex (FR-017, SC-003, US4)

**Checkpoint**: Run
`go test -race -count=1 ./openaicompat ./embed ./llm`.
User Story 4 is independently demonstrable once it passes.

---

## Phase 7: Polish & Cross-Cutting Concerns

**Purpose**: Documentation, quality gates, and release surface.

- [ ] T039 [P] Update `README.md` provider configuration and environment-variable tables for `openai-compatible`: the two sections, endpoint precedence, and the credential chain, with no key value examples; also fix every provider-list mention this feature makes stale — line 5 ("pluggable LLM providers (Ollama, Vertex AI)"), the `doctor` description (line ~375), Semantic Search Setup (line ~558), and the Architecture section (line ~774 and ~779-783: the `llm/` synthesizer list must add `OpenAICompatibleSynthesizer`, the `embed/` file list must add `openai_compatible.go`, and the `openaicompat/` package must be listed)
- [ ] T040 [P] Update `AGENTS.md` provider configuration, environment-variable precedence, the `openaicompat` package in the architecture layout, active technologies, the Spec Organization list (add `017-openai-compatible-providers`), the Recent Changes section, and the stale local Gaze example (`--max-gaze-crapload=18 --min-contract-coverage=70`), refreshing it to match `.github/workflows/ci.yml` (`--max-crapload=48 --max-gaze-crapload=45`, no coverage floor); also update every provider-list mention this feature makes stale — the Project Overview provider list, the Key Patterns "Pluggable providers" bullet ("(ollama, vertex)"), and the Technical Guardrails "Local-Only Processing" sentence ("Cloud providers (Vertex AI) are opt-in via config.yaml")
- [ ] T041 [P] Verify GoDoc on every new exported symbol in `openaicompat/`, `embed/openai_compatible.go`, `llm/openai_compatible.go`, and the two `UnavailableReasoner` interfaces
- [ ] T042 Run the CI-parity gate using the exact commands in `.github/workflows/ci.yml`: `go mod download`, `go build -v ./...`, `go vet ./...`, `go test -race -v -count=1 -coverprofile=coverage.out ./...`, and `gaze crap --format=json --coverprofile=coverage.out --max-crapload=48 --max-gaze-crapload=45 ./...`; also confirm `gofmt -l openaicompat embed llm` reports no files
- [ ] T043 Execute quickstart validation sections 1 through 5 in `specs/017-openai-compatible-providers/quickstart.md`, including the closed-port degradation smoke test and the credential grep, and confirm every expected outcome
- [ ] T044 File the website documentation sync issue: first run `gh issue list --repo unbound-force/website --search 'dewey provider' --state open` and reference a matching existing issue (the website repo already tracks dewey provider documentation in unbound-force/website#113) instead of filing a duplicate; otherwise file it with `gh issue create --repo unbound-force/website` titled `docs: sync dewey openai-compatible provider documentation`, referencing the `README.md` provider section and the `config.yaml` endpoint and credential fields
- [ ] T045 [P] At implementation time, search before filing: run `gh issue list --search 'openai-compatible gateway' --state open` and reference an existing match instead of filing a duplicate; otherwise file the blog coverage issue with `gh issue create --label blog --title "blog: point dewey at any OpenAI-compatible gateway (OpenRouter, LiteLLM, vLLM)"` covering the problem (frontier-model synthesis without local Ollama), the approach (third provider on the OpenAI v1 wire protocol, environment-only credentials), and evidence from the quickstart degradation and retry scenarios
- [ ] T046 [P] At implementation time, search before filing: run `gh issue list --search 'OpenAI-compatible gateway tutorial' --state open` and reference an existing match instead of filing a duplicate; otherwise file the tutorial coverage issue with `gh issue create --label tutorial --title "tutorial: switching dewey to an OpenAI-compatible gateway"` covering: choose a gateway, set `config.yaml` provider/model/endpoint, resolve the API key through the environment chain, verify with `dewey doctor`, and understand degradation and 429 retry behavior
- [ ] T047 [P] At implementation time, search before filing: run `gh issue list --repo unbound-force/dewey --search 'doctor vertex ollama embedding layer' --state open` and reference an existing match instead of filing a duplicate; otherwise file the follow-up issue for the pre-existing `dewey doctor` gap where the embedding layer treats Vertex as Ollama, with `gh issue create --repo unbound-force/dewey` referencing the plan.md risk note ("Pre-existing gap observed, out of scope") and spec 017; do not fix the gap in this feature (D8, FR-017)

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies.
- **Foundational (Phase 2)**: Depends on Setup; blocks all user
  stories.
- **User Stories (Phase 3+)**: Depend on Foundational completion.
  US1 and US2 touch different packages and may proceed in parallel.
  US3 depends on the US1 synthesis provider and the US2 embedding
  provider for its diagnostics and degradation wiring. US4 depends on
  both providers and on the Foundational credential chain.
- **Polish (Phase 7)**: Depends on every delivered user story.

### User Story Dependencies

- **US1 (P1)**: Independent after Foundational. Delivers the MVP.
- **US2 (P2)**: Independent of US1; different package and files.
- **US3 (P2)**: Depends on US1 and US2 implementations for
  `UnavailableReasoner` values; tests and diagnostics touch only
  tool, CLI, and startup files.
- **US4 (P3)**: Depends on US1, US2, and Foundational; adds only a new
  cross-package test file and a verification run.

### Within Each User Story

- Contract tests are written first and must fail before the
  implementation task completes.
- Configuration and capability interfaces precede the provider
  implementation.
- The provider implementation precedes factory registration.
- Factory registration precedes factory and configuration tests.
- Each story finishes at its checkpoint before the next priority
  begins.

### Parallel Opportunities

- Setup: T002.
- Foundational implementations: T003, T004, T005, T006.
- Foundational contract tests: T009, T010, T011, T012, T013, T014.
- US1: T019 and T020 (written first), then T015 and T016.
- US2: T025 and T026 (written first), then T021 and T022.
- US3: T032, T033, T034, T035, T036 (written first), then T027,
  T028, T029, T030, T031.
- Polish: T039, T040, T041.

---

## Parallel Example: Foundational Transport

```bash
# Implement the independent transport pieces together:
Task: "Implement endpoint normalization in openaicompat/endpoint.go"
Task: "Implement key resolution in openaicompat/auth.go"
Task: "Implement retry delay in openaicompat/retry.go"
Task: "Implement redacted excerpts in openaicompat/errors.go"

# After client.go and availability.go land, run the contract suites:
Task: "Write openaicompat/endpoint_test.go"
Task: "Write openaicompat/auth_test.go"
Task: "Write openaicompat/retry_test.go"
Task: "Write openaicompat/errors_test.go"
Task: "Write openaicompat/client_test.go"
Task: "Write openaicompat/availability_test.go"
```

---

## Parallel Example: User Story 3

```bash
# Independent wiring files:
Task: "Surface unavailable reason in tools/semantic.go"
Task: "Surface unavailable reason in tools/compile.go"
Task: "Surface unavailable reason in tools/curate.go"
Task: "Make dewey doctor provider-aware in cli.go"
Task: "Make startup logging provider-aware in main.go"

# Independent test files:
Task: "Extend tools/semantic_test.go"
Task: "Extend tools/compile_test.go"
Task: "Extend tools/curate_test.go"
Task: "Extend cli_test.go"
Task: "Extend main_test.go"
```

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Complete Phase 1: Setup.
2. Complete Phase 2: Foundational transport.
3. Complete Phase 3: User Story 1.
4. **STOP and VALIDATE**: run the US1 checkpoint and the stub-gateway
   compile flow.
5. Deploy or demo synthesis through a gateway.

### Incremental Delivery

1. Setup plus Foundational gives one audited transport surface.
2. US1 delivers gateway synthesis (MVP).
3. US2 adds gateway embeddings without touching US1 files.
4. US3 adds retry visibility and graceful degradation.
5. US4 proves backward compatibility and credential isolation.
6. Polish closes the documentation and quality gates.

### Parallel Team Strategy

1. One engineer completes Setup and Foundational.
2. After Foundational, one engineer takes US1 (`llm`) and another
   takes US2 (`embed`); their files do not overlap.
3. US3 and US4 follow once both providers exist.

---

## Notes

- `[P]` tasks touch different files and have no dependency on an
  incomplete task in the same group.
- `[Story]` labels map every feature task to its user story.
- Mark a task `[x]` immediately when its own verification passes.
- Do not modify the gate thresholds or CI flags to make a task pass.
- Commit after each task or logical group.
- Stop at each checkpoint to validate the story independently.
