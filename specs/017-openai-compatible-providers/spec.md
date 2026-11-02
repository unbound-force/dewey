# Feature Specification: OpenAI-Compatible API Providers

**Feature Branch**: `017-openai-compatible-providers`

**Created**: 2026-11-02

**Status**: Draft

**Issue**: [#141](https://github.com/unbound-force/dewey/issues/141)

**Input**: User description: "Support OpenAI-compatible gateways for
embedding and synthesis"

## User Scenarios & Testing *(mandatory)*

### User Story 1 - OpenAI-compatible synthesis (Priority: P1)

A developer wants to synthesize compiled knowledge with a frontier model
served through an OpenAI-compatible gateway (for example ENMaas) instead
of a local Ollama model. They set `synthesis.provider` to
`openai-compatible`, point `synthesis.endpoint` at the gateway, and
provide the API key through an environment variable. `dewey compile` and
curation then use the gateway without any code change.

**Why this priority**: Synthesis quality is the primary motivation for
remote providers; local models produce weaker compiled articles. This
story alone delivers the main value.

**Independent Test**: Configure an OpenAI-compatible synthesis provider
against a stub gateway that returns a known completion. Run `dewey
compile` with stored learnings. Verify the request path, the model
field, and that the returned text is persisted as the compiled article.

**Acceptance Scenarios**:

1. **Given** `config.yaml` contains `synthesis.provider:
   openai-compatible`, `synthesis.model: enmaas/rits/zai-org/glm-5-3`,
   `synthesis.endpoint: https://gateway.example/v1`, **And** the
   synthesis API key is present in the environment, **When** `dewey
   compile` runs, **Then** Dewey sends `POST
   https://gateway.example/v1/chat/completions` with the configured
   model and a user message, and persists the returned assistant text.
2. **Given** the configured model identifier contains slashes and
   hyphens, **When** a synthesis request is sent, **Then** the
   identifier is sent verbatim in the request `model` field.
3. **Given** `synthesis.provider: openai-compatible` but no
   `synthesis.endpoint`, **When** the synthesizer factory is called,
   **Then** it returns an error naming the missing `synthesis.endpoint`
   field.
4. **Given** `synthesis.provider: openai-compatible` with a valid
   `synthesis.endpoint` but no API key in the environment, **When**
   the synthesizer factory is called, **Then** it constructs the
   provider successfully — the key is optional for unauthenticated
   self-hosted gateways (FR-008) — and when the endpoint later
   answers HTTP 401/403 from the availability probe or a request,
   the returned error names the expected environment variable and
   never suggests storing the key in `config.yaml`.
5. **Given** the gateway returns HTTP 401, **When** synthesis runs,
   **Then** the returned error includes the status code and a bounded
   response excerpt, and contains no credential material.
6. **Given** the gateway returns HTTP 200 with no assistant content,
   **When** synthesis runs, **Then** Dewey returns a descriptive error
   rather than an empty article.
7. **Given** `synthesis.provider: openai-compatible` but no
   `synthesis.model`, **When** the synthesizer factory is called,
   **Then** it returns an error naming the missing `synthesis.model`
   field.

---

### User Story 2 - OpenAI-compatible embeddings (Priority: P2)

A developer without Ollama wants semantic search powered by an
OpenAI-compatible embedding model (for example OpenRouter). They
configure `embedding.provider: openai-compatible`, an embedding model,
an endpoint, and the key in the environment. `dewey index` then produces
embeddings for every indexed page and semantic search works normally.

**Why this priority**: Unlocks semantic search for environments that
cannot run Ollama. It is independently valuable but secondary to
synthesis quality.

**Independent Test**: Configure an OpenAI-compatible embedding provider
against a stub gateway that returns deterministic vectors. Run `dewey
index`. Verify the request path and that vectors are stored and returned
in input order.

**Acceptance Scenarios**:

1. **Given** `config.yaml` contains `embedding.provider:
   openai-compatible`, `embedding.model:
   openrouter/amazon/titan-embed-text-v2`, `embedding.endpoint:
   https://openrouter.ai/api/v1`, **And** the embedding API key is
   present in the environment, **When** `dewey index` runs, **Then**
   Dewey sends `POST
   https://openrouter.ai/api/v1/embeddings` with the configured
   model and the input texts, and stores one vector per input.
2. **Given** a batch of N texts, **When** embeddings are generated,
   **Then** a single request carries all N inputs and exactly N vectors
   are returned in the original input order.
3. **Given** the gateway returns vectors whose `index` fields are out of
   order, **When** the response is parsed, **Then** vectors are restored
   to input order before storage.
4. **Given** the gateway returns a different number of vectors than
   inputs, **When** the response is parsed, **Then** Dewey returns a
   descriptive error.
5. **Given** an empty input list, **When** batch embedding is requested,
   **Then** Dewey returns an empty result without making a network call.
6. **Given** `embedding.provider: openai-compatible` but no
   `embedding.model`, **When** the embedder factory is called, **Then**
   it returns an error naming the missing `embedding.model` field.

---

### User Story 3 - Resilience and graceful degradation (Priority: P2)

An operator points Dewey at a shared gateway that occasionally rate
limits or is briefly unreachable. Dewey retries rate-limited requests
automatically and, when the gateway stays down, keeps all keyword and
graph tools working while reporting semantic features as unavailable.

**Why this priority**: Remote gateways are shared and imperfect. Without
retry and degradation, a transient 429 would break indexing or
compilation, and an outage could make the server appear broken.

**Independent Test**: Use a stub gateway that returns 429 twice and
then succeeds with `Retry-After`. Verify the request eventually
succeeds. Then use an unreachable endpoint and verify non-semantic MCP
tools still respond.

**Acceptance Scenarios**:

1. **Given** the gateway returns HTTP 429 with `Retry-After: 2`,
   **When** a request is made, **Then** Dewey waits approximately the
   indicated delay and retries, and succeeds if the next attempt
   succeeds.
2. **Given** the gateway keeps returning HTTP 429, **When** the retry
   budget is exhausted, **Then** Dewey returns a descriptive rate-limit
   error after the initial attempt plus at most five retries.
3. **Given** the gateway omits `Retry-After`, **When** a 429 is retried,
   **Then** Dewey applies exponential backoff starting at one second,
   capped at sixty seconds, with randomized jitter.
4. **Given** the caller cancels the context during a backoff wait,
   **When** the wait is in progress, **Then** Dewey returns promptly
   with the context error instead of completing the wait.
5. **Given** an unreachable OpenAI-compatible endpoint at server start,
   **When** a semantic search is requested, **Then** the tool returns an
   actionable error, while keyword search, page navigation, and graph
   tools continue to return results.

---

### User Story 4 - Compatibility and separate gateways (Priority: P3)

An existing user upgrades Dewey with `ollama` or `vertex` configuration
and sees no behavioural change. A second user points embedding and
synthesis at two different gateways that require two different keys, and
both work in the same process.

**Why this priority**: Protects the installed base and supports the
common real-world case of mixed gateways. It is a safety and ergonomics
story rather than a new capability.

**Independent Test**: Run the existing provider test suites unchanged.
Then configure embedding and synthesis with different endpoints and
different keys and verify both requests authenticate correctly.

**Acceptance Scenarios**:

1. **Given** an existing `embedding.provider: ollama` configuration,
   **When** Dewey starts, **Then** it constructs the Ollama embedder
   exactly as before.
2. **Given** an existing `synthesis.provider: vertex` configuration,
   **When** `dewey compile` runs, **Then** it constructs the Vertex
   synthesizer exactly as before.
3. **Given** embedding and synthesis use different gateways, **When**
   each API key is supplied through its provider-scoped environment
   variable, **Then** each request uses its own key.
4. **Given** `embedding.provider: unsupported`, **When** the factory is
   called, **Then** the error lists `ollama`, `vertex`, and
   `openai-compatible` as supported values.

---

### Edge Cases

- Endpoint values with a trailing slash or without a URL scheme. Dewey
  MUST normalize them before appending the operation path.
- An API key environment variable that is set but empty. Dewey MUST
  treat it as unset.
- A gateway that returns HTTP 200 with malformed JSON or an unexpected
  payload shape. Dewey MUST return a parse error that names the
  provider.
- A gateway that returns an error body larger than the excerpt limit.
  Dewey MUST truncate the excerpt and MUST NOT include credentials.
- A self-hosted gateway that does not require authentication. This is
  supported: no key is required and no `Authorization` header is sent
  when none resolves (FR-008).
- An endpoint whose scheme is normalized to `http://` on a
  non-loopback host while a key resolves. Dewey MUST warn that the
  credential would transit cleartext (FR-024); loopback hosts are
  exempt because local servers may use HTTP.
- Very large embedding batches that exceed a gateway's per-request
  limit. Dewey MUST surface the gateway error rather than silently
  dropping inputs.
- A gateway that redirects to another host. The HTTP client follows
  redirects, but the `Authorization` header MUST NOT be forwarded across
  hosts.
- Concurrent calls to `Available()` and `Embed` or `Synthesize`. The
  implementation MUST be safe for concurrent use.
- A prompt or document containing non-ASCII text. It MUST be sent as
  UTF-8 JSON without alteration.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: Dewey MUST accept `openai-compatible` as a value for
  `embedding.provider` in `config.yaml`.
- **FR-002**: Dewey MUST accept `openai-compatible` as a value for
  `synthesis.provider` in `config.yaml`.
- **FR-003**: The OpenAI-compatible embedder MUST send `POST
  {endpoint}/embeddings` with a JSON body containing `model` and an
  `input` array, and MUST parse the response `data` entries into
  `float32` vectors.
- **FR-004**: The OpenAI-compatible synthesizer MUST send `POST
  {endpoint}/chat/completions` with a JSON body containing `model` and a
  `messages` array, and MUST extract the assistant text from the first
  returned choice.
- **FR-005**: Dewey MUST pass the configured model identifier to the
  gateway verbatim, without parsing, splitting, or rewriting slashes and
  hyphens.
- **FR-006**: Dewey MUST read the API key from environment variables
  only. The key MUST NOT be accepted from, or written to, `config.yaml`.
- **FR-007**: Dewey MUST resolve the API key in this order: the
  provider-scoped variable (`DEWEY_EMBEDDING_API_KEY` for embedding,
  `DEWEY_SYNTHESIS_API_KEY` for synthesis), then the shared
  `DEWEY_API_KEY`, then `OPENROUTER_API_KEY` when the endpoint host is
  `openrouter.ai`. The host comparison uses the parsed URL's hostname
  with the port stripped, compared case-insensitively; subdomains do
  not match.
- **FR-008**: When an OpenAI-compatible endpoint requires
  authentication — an HTTP 401 or 403 from the availability probe or
  a request — and no API key is available, Dewey MUST return a clear
  error naming the expected environment variables and never suggest
  storing the key in `config.yaml`. The API key is optional, matching
  the repository's established pattern for optional service
  credentials (the GitHub source token chain resolves env var → gh
  auth token → unauthenticated; credentials are runtime-only and
  never persisted). The `Authorization` header is sent only when a
  key resolves from the FR-007 chain (`DEWEY_EMBEDDING_API_KEY` /
  `DEWEY_SYNTHESIS_API_KEY`, then `DEWEY_API_KEY`, then
  `OPENROUTER_API_KEY` on `openrouter.ai` hosts). When no key
  resolves, the header is omitted so unauthenticated self-hosted
  servers (vLLM, LiteLLM, private gateways) work. The factory errors
  only on a missing endpoint (FR-009), never on a missing key alone;
  the factory cannot know whether an endpoint enforces auth, so the
  clear error fires from the availability probe or request path upon
  the 401/403.
- **FR-009**: The factory MUST return a clear error naming
  `embedding.endpoint` or `synthesis.endpoint` when the endpoint is
  missing for an OpenAI-compatible provider.
- **FR-010**: Dewey MUST treat the configured endpoint as the API base
  URL, remove a trailing slash and any trailing operation path, and
  append `/embeddings` or `/chat/completions` exactly once, so the
  final URL never contains a doubled path.
- **FR-011**: On HTTP 429, Dewey MUST retry after the delay given by an
  integer-seconds `Retry-After` header when present, otherwise with
  exponential backoff from a one-second base, capped at sixty seconds,
  including randomized jitter. A missing, negative, zero, non-numeric,
  or HTTP-date `Retry-After` value MUST be treated as absent, falling
  back to exponential backoff.
- **FR-012**: Dewey MUST perform the initial request plus at most five
  retries before returning a rate-limit error.
- **FR-013**: Dewey MUST honor context cancellation during retry waits
  and MUST return the context error promptly.
- **FR-014**: Each OpenAI-compatible provider MUST cache its
  availability result for thirty seconds and MUST be safe for
  concurrent use. Availability is probed with a cheap, authenticated
  HTTP GET to `{endpoint}/models` — the OpenAI wire protocol's
  standard model-list route and the direct analog of the Ollama
  health check (GET /api/tags, per the established provider pattern:
  a cheap HTTP GET whose 2xx confirms the service is ready). Any 2xx
  response means available; connection errors and 401/403 responses
  mean unavailable (a 401/403 with no key configured triggers
  FR-008's clear authentication error). The probe does not require
  the configured model to appear in the `/models` list — gateway
  aliases, model rewriting (LiteLLM), and lazy model loading (vLLM)
  make membership checks unreliable across heterogeneous gateways;
  model-level errors surface at request time as FR-020 status-code
  errors.
- **FR-015**: When an OpenAI-compatible provider is unavailable, Dewey
  MUST degrade gracefully: keyword search, page navigation, graph tools,
  and all other non-semantic tools MUST continue to work, and semantic
  tools MUST return an actionable error.
- **FR-016**: Both factories (`NewEmbedderFromConfig` and
  `NewSynthesizerFromConfig`) MUST register the `openai-compatible`
  provider and MUST include it in the supported-provider list returned
  for unknown values.
- **FR-017**: Dewey MUST preserve existing behaviour for `ollama` and
  `vertex`, including existing environment variables, defaults, and
  configuration files.
- **FR-018**: The implementation MUST be pure Go with no CGO and MUST
  use direct HTTP calls following the existing Vertex provider pattern.
  It MUST NOT add a vendor SDK dependency.
- **FR-019**: API keys MUST NOT appear in logs, error messages, audit
  output, or any generated artifact.
- **FR-020**: On a non-success HTTP response, Dewey MUST return an error
  containing the status code and a bounded, credential-free excerpt of
  the response body. Every occurrence of the resolved key MUST be
  replaced with `[REDACTED]` before the excerpt is truncated, so a key
  straddling the truncation boundary cannot leak a partial prefix. The
  excerpt MUST have all C0 control characters (newlines, tabs, carriage
  returns, backspaces, and the rest of the C0 range) and ESC-initiated
  escape sequences stripped or escaped so gateway-controlled bytes
  cannot forge log lines or corrupt terminal rendering.
- **FR-021**: Synthesis requests MUST be non-streaming and MUST NOT rely
  on tool calling, function calling, or structured-output features.
- **FR-022**: OpenAI-compatible endpoint resolution MUST follow each
  code path's documented precedence, preserving the repository's
  documented, intentional asymmetry: embedding is env-first
  (`DEWEY_EMBEDDING_ENDPOINT` > `config.yaml` `embedding.endpoint`)
  and synthesis is config-first (`config.yaml` `synthesis.endpoint` >
  `DEWEY_SYNTHESIS_ENDPOINT`), because `ReadSynthesisConfig()` checks
  config file values first and returns early. For openai-compatible
  endpoints the `OLLAMA_HOST` tier does not apply (it is an
  Ollama-specific fallback); the endpoint must come from the code
  path's env var or `config.yaml` and is otherwise a missing-endpoint
  error per FR-009 — there is no localhost default for gateway
  endpoints.
- **FR-023**: The embedder MUST return exactly one vector per input in
  the original input order, reordering by the response `index` field
  when the gateway returns entries out of order.
- **FR-024**: `dewey doctor`, startup logging, and the shared
  indexing helper used by `dewey index`, `dewey reindex`, `dewey
  compile`, `dewey curate`, and `dewey lint` MUST recognize the
  `openai-compatible` provider: report configuration completeness and
  probe status without printing credentials, list `openai-compatible`
  in the unknown-provider message, and emit the `ollama pull` hint
  only for Ollama at every warning site. Startup logging and `dewey
  doctor` MUST also warn when a resolved API key would transit
  `http://` to a non-loopback host, and when an `api_key` field is
  present in `config.yaml` (it is ignored; keys are environment-only
  per FR-006).
- **FR-025**: OpenAI-compatible HTTP timeouts MUST be
  capability-scoped: embedding requests use a thirty-second client
  timeout, synthesis requests use a three-hundred-second client
  timeout (matching the Vertex synthesizer; see research.md R12 for
  the divergence from the 013 Ollama contract), and the availability
  probe uses its own one-second timeout (FR-014).

### Key Entities

- **Provider Configuration**: The `embedding` and `synthesis` sections
  of `config.yaml`. Carries provider name, model, endpoint, and the
  existing optional project, region, and chunk-size fields.
- **Credential Reference**: The environment variable name and value that
  authenticates gateway requests. The value is transient and is never
  persisted.
- **Embedding Vector**: A floating-point vector produced for one input
  text, associated with its input position.
- **Completion**: The assistant text returned by a synthesis request for
  one prompt.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: A user can switch compilation from a local model to an
  OpenAI-compatible gateway by editing configuration and setting one
  environment variable, with no code change and under ten minutes of
  configuration effort.
- **SC-002**: After indexing with an OpenAI-compatible embedder, 100% of
  indexed pages have embeddings and semantic search returns results.
- **SC-003**: 100% of existing `ollama` and `vertex` configurations
  continue to work with zero configuration changes after upgrade.
- **SC-004**: When a gateway is unreachable, 100% of non-semantic tools
  continue to return results, and semantic tools return an actionable
  error within one second.
- **SC-005**: At least 95% of simulated rate-limit bursts that clear
  within sixty seconds complete without user intervention.
- **SC-006**: Credential material appears zero times in logs, error
  output, and generated artifacts across the test suite.
- **SC-007**: 100% of missing-endpoint and missing-model factory
  failures produce an error that names the exact configuration field
  to set, and every authentication failure (a 401/403 with no key)
  names the exact environment variable to set.

## Assumptions

- Gateways implement the OpenAI v1 request and response shapes for
  `/chat/completions` and `/embeddings`.
- Model identifiers are opaque strings, including provider-prefixed
  identifiers with multiple slashes.
- The endpoint can be a public gateway or a private self-hosted server;
  HTTPS is expected for public gateways and HTTP is allowed for local
  servers.
- The configured endpoint is the API base URL. Dewey appends the
  operation path and does not expect the endpoint to include it.
- API key resolution uses the layered precedence in FR-007; this mirrors
  the issue's examples and supports two gateways in one process.
- The existing thirty-second availability cache and retry constants are
  reused so behaviour stays consistent with the Vertex providers.
- Provider selection remains configuration-only; no new MCP tools or CLI
  commands are introduced.
- Remote gateways are an opt-in extension, consistent with the Vertex
  precedent in spec 016. Ollama remains the default.
- `config.yaml` keeps the existing `embedding` and `synthesis` section
  structure; no new top-level configuration file is introduced.
- The existing `Embedder` and `Synthesizer` interfaces are sufficient;
  no interface change is required.

## Scope

**In scope**:

- An OpenAI-compatible embedding provider and synthesis provider.
- Factory registration and configuration reading for the new provider.
- Retry, availability caching, and graceful degradation.
- Provider-aware diagnostics: `dewey doctor`, startup logging, and the
  shared indexing helper's unavailability warning recognize
  `openai-compatible` without credential exposure.
- Automated tests that use stub gateways and need no live service, plus
  documentation updates.

**Out of scope**:

- Streaming responses, tool calling, function calling, and JSON-mode
  response formats.
- New MCP tools or CLI commands.
- Automatic discovery of gateways or model catalogues.
- Storing API keys in configuration files or the knowledge store.
- Changes to the `ollama` or `vertex` implementations.
- Anthropic-native or other non-OpenAI wire protocols.

## Dependencies

- The existing `embed.Embedder` and `llm.Synthesizer` interfaces.
- `embed.NewEmbedderFromConfig` and `llm.NewSynthesizerFromConfig`.
- `embed.ReadEmbeddingConfig` and `llm.ReadSynthesisConfig`.
- The existing HTTP retry and availability-cache patterns from the
  Vertex providers.
- Standard-library HTTP, JSON, and context support already used by the
  existing providers.

## Constitution Alignment: Local-Only Processing

The constitution requires that no data leave the developer's machine for
core functionality. OpenAI-compatible gateways are an opt-in extension,
exactly like Vertex in spec 016: Ollama stays the default, and every
non-semantic capability works without any remote provider. A user who
configures a gateway deliberately sends content to that endpoint. The
implementation MUST NOT contact any remote endpoint unless the user has
selected `openai-compatible` in `config.yaml`.

## Clarifications

- Q: Is a key required for self-hosted servers? -> A: No — the API
  key is optional, matching the repository's established pattern for
  optional service credentials (the GitHub source token chain
  resolves env var → gh auth token → unauthenticated; credentials
  are runtime-only and never persisted). The Authorization header is
  sent only when a key resolves from the FR-007 chain
  (DEWEY_EMBEDDING_API_KEY / DEWEY_SYNTHESIS_API_KEY, then
  DEWEY_API_KEY, then OPENROUTER_API_KEY on openrouter.ai hosts).
  When no key resolves, the header is omitted so unauthenticated
  self-hosted servers (vLLM, LiteLLM, private gateways) work. The
  factory errors only on a missing endpoint (FR-009), never on a
  missing key alone. An endpoint that requires authentication
  answers 401/403, and FR-008's clear error ("endpoint requires
  authentication and no API key is available", naming the env vars
  to set) fires from the availability probe or request path upon
  that 401/403 — not at factory time, since the factory cannot know
  whether a given endpoint enforces auth. (Dewey-resolved from
  specs/001-core-implementation/spec)
- Q: Which probe determines availability? -> A: The probe is a
  cheap, authenticated HTTP GET to {endpoint}/models — the OpenAI
  wire protocol's standard model-list route and the direct analog of
  the Ollama health check (GET /api/tags, per the established
  provider pattern: a cheap HTTP GET whose 2xx confirms the service
  is ready). Any 2xx response means available; connection errors and
  401/403 responses mean unavailable (a 401/403 with no key
  configured triggers FR-008's clear authentication error). The
  probe does not require the configured model to appear in the
  /models list — gateway aliases, model rewriting (LiteLLM), and
  lazy model loading (vLLM) make membership checks unreliable across
  heterogeneous gateways; model-level errors surface at request time
  as FR-020 status-code errors. The availability result is cached
  for thirty seconds and the probe is safe for concurrent use
  (FR-014). (Dewey-resolved from specs/007-ollama-autostart/research)
- Q: Should synthesis endpoints be env-first? -> A: No — synthesis
  endpoints stay config-first, preserving the repository's
  documented, intentional precedence asymmetry. The openai-compatible
  provider must follow each code path's existing documented chain:
  embedding is env-first (DEWEY_EMBEDDING_ENDPOINT > config.yaml
  embedding.endpoint > OLLAMA_HOST > default), while synthesis is
  config-first (config.yaml synthesis.endpoint >
  DEWEY_SYNTHESIS_ENDPOINT > OLLAMA_HOST > default), because
  ReadSynthesisConfig() checks config file values first and returns
  early. For openai-compatible endpoints the OLLAMA_HOST tier does
  not apply (it is an Ollama-specific fallback); the endpoint must
  come from the code path's env var or config.yaml and is otherwise
  a missing-endpoint error per FR-009 — there is no localhost
  default for gateway endpoints. (Dewey-resolved from
  openspec/changes/synthesis-endpoint-env/
  specs/synthesis-endpoint-resolution)
