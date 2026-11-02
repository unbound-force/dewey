# Research: OpenAI-Compatible API Providers

**Feature**: `017-openai-compatible-providers` |
**Date**: 2026-11-02 | **Spec**: [spec.md](./spec.md)

All open questions from the specification were resolved during
`/speckit.clarify`; no clarification markers remain. This document
records the technical decisions that turn those resolutions into an
implementation shape.

## R1: Shared transport package versus duplicated providers

- **Decision**: Add a new leaf package `openaicompat` that owns base-URL
  normalization, API-key resolution, authenticated JSON POST, 429 retry
  with backoff, credential-free error excerpts, and the cached
  `/models` probe. `embed` and `llm` each keep their own
  `ProviderConfig`, request and response types, parsing, and factory.
- **Rationale**: The FR-007 credential chain is security-sensitive and
  must not diverge between embedding and synthesis. Retry, excerpt,
  normalization, and probe logic are identical for both capabilities
  and total roughly 150 lines. Duplicating that surface twice invites
  drift and doubles the review surface for the security rules. Spec
  016 deliberately duplicated `ProviderConfig` to decouple
  configuration; that decision is preserved because configuration and
  factories stay per package. The `synthesis-endpoint-env` design left
  extraction open for the third-provider case; a third gateway
  protocol has now arrived.
- **Alternatives considered**:
  - Duplicate everything in `embed` and `llm`, mirroring Vertex
    (rejected: two copies of the credential chain, ~300 lines total).
  - Put the client in `embed` and import it from `llm` (rejected:
    synthesis would depend on embedding provider internals).
  - Put it under `internal/` (viable, but inconsistent with the flat
    public layout and the manifest's exported-package scan).

## R2: Credential resolution and storage

- **Decision**: Resolve keys from the environment only, in this order:
  `DEWEY_EMBEDDING_API_KEY` or `DEWEY_SYNTHESIS_API_KEY` by capability,
  then `DEWEY_API_KEY`, then `OPENROUTER_API_KEY` when the endpoint
  host is exactly `openrouter.ai`. Empty values count as unset. No
  `api_key` field is added to configuration structs or YAML.
- **Rationale**: FR-006 and FR-007 make the environment the only
  accepted source and make the key transient. The host check is a
  convenience for OpenRouter users and does not weaken the explicit
  variables. Omitting the header when no key resolves supports
  unauthenticated self-hosted servers (FR-008 clarification). Two
  guardrails follow: when normalization leaves a key transiting
  `http://` to a non-loopback host, startup and `dewey doctor` warn
  that the credential would transit cleartext (FR-024); and a key
  present in `config.yaml` is ignored with a startup and doctor
  warning, because a silently ignored secrets-on-disk field is a
  hazard.
- **Alternatives considered**:
  - A `config.yaml` key field (rejected: explicitly forbidden).
  - A required key for every gateway (rejected: breaks vLLM, LiteLLM,
    and private unauthenticated servers).
  - Subdomain matching for OpenRouter (rejected: the spec names the
    host `openrouter.ai`; exact match is predictable).

## R3: Availability probe and actionable reasons

- **Decision**: `Available()` sends an authenticated
  `GET {base}/models` with a one-second timeout, treats any 2xx as
  available, caches the boolean for thirty seconds, and records a
  credential-free reason on failure. 401 and 403 with no resolved key
  produce the FR-008 authentication message; 401 or 403 with a key
  produce a "verify the key" message. The reason is exposed through an
  optional `UnavailableReason()` capability interface implemented by
  both providers and asserted by consumers.
- **Rationale**: `/models` is the OpenAI wire protocol's standard
  model-list route and the direct analog to Ollama's `/api/tags`
  health check. SC-004 requires an actionable error within one second,
  so the probe timeout is one second. Model membership is not checked
  because aliases, LiteLLM rewriting, and lazy vLLM loading make it
  unreliable. The optional interface avoids changing `Embedder` or
  `Synthesizer`, which the spec's assumptions require to stay stable.
  The one-second timeout is an accepted trade-off: while the cache
  reports unavailable, semantic tools degrade to keyword-only
  (FR-015) and the index path skips embedding generation rather than
  attempting requests, so a healthy remote gateway whose `/models`
  route consistently exceeds one second stays keyword-only until a
  post-expiry probe succeeds; SC-004's one-second requirement takes
  precedence.
- **Alternatives considered**:
  - A five-second probe like Vertex (rejected: violates SC-004).
  - Checking that the configured model appears in `/models`
    (rejected: unreliable across gateways).
  - Adding an error return to `Available()` (rejected: interface
    change with wide blast radius).

## R4: Endpoint normalization and configuration precedence

- **Decision**: Treat the configured endpoint as the API base URL.
  Prepend `http://` when no scheme is present, remove trailing
  slashes, and append exactly one operation path. For
  `openai-compatible`, embedding is env-first and synthesis is
  config-first, matching the documented per-path asymmetry. The
  `OLLAMA_HOST` and localhost defaults apply only to Ollama; a missing
  gateway endpoint is a factory error naming the configuration field.
- **Rationale**: FR-010 and FR-022 require exactly this behavior. The
  asymmetry is intentional and documented in AGENTS.md; changing it
  would alter Ollama behavior (FR-017).
- **Alternatives considered**:
  - A unified env-first rule for both capabilities (rejected: changes
    documented synthesis behavior).
  - Accepting endpoints that already include `/v1` and appending
    blindly (rejected: duplicate paths; the contract states the
    endpoint is the base URL).
  - Defaulting a gateway to localhost (rejected: there is no sensible
    local gateway default and it would mask configuration errors).

## R5: Request and response shapes

- **Decision**: Embeddings send
  `{"model": "<verbatim>", "input": ["...", ...]}` to
  `POST {base}/embeddings` and read `data[].embedding` plus
  `data[].index`. Synthesis sends
  `{"model": "<verbatim>", "messages": [{"role": "user",
  "content": "<prompt>"}], "stream": false}` to
  `POST {base}/chat/completions` and reads
  `choices[0].message.content`. `max_tokens` is omitted.
- **Rationale**: FR-003 through FR-005 and FR-021 define these shapes.
  Omitting `max_tokens` avoids gateway-specific output limits that
  reject a forced value while still letting each model use its own
  default. Model identifiers are opaque strings and are never split or
  rewritten.
- **Alternatives considered**:
  - Sending `max_tokens: 16000` like Vertex (rejected: many gateways
    reject values above a model's output limit).
  - Streaming with `stream: true` (rejected: explicitly out of scope).
  - Tool or structured-output modes (rejected: explicitly out of
    scope).

## R6: Response ordering and validation

- **Decision**: Reorder embedding entries by their `index` field before
  conversion to `float32`. Return an error when the entry count differs
  from the input count, when an index is duplicated, or when an index
  falls outside the input range. Return `nil, nil` for an empty input
  batch without a network call. Return a named error when the first
  chat choice has empty or missing assistant content.
- **Rationale**: FR-023 requires one vector per input in input order,
  and US2 scenarios 3 through 5 require explicit handling of reorder,
  count mismatch, and empty batches. Erroring on empty synthesis
  content prevents persisting an empty compiled article (US1-6).
- **Alternatives considered**:
  - Trusting gateway ordering (rejected: the spec explicitly covers
    out-of-order `index` values).
  - Sorting by index but silently truncating extras (rejected: the
    spec requires surfacing a descriptive error).
  - Persisting an empty article (rejected: violates US1-6).

## R7: Retry and backoff

- **Decision**: Retry only on HTTP 429. Use an integer-seconds
  `Retry-After` value capped at sixty seconds; otherwise exponential
  backoff from one second, doubling per attempt, with plus or minus
  25 percent jitter, capped at sixty seconds. Run the initial request
  plus at most five retries. Return the context error immediately when
  the context is canceled during a wait.
- **Rationale**: FR-011 through FR-013 restate the Vertex policy, and
  the spec assumptions ask for consistency with Vertex behavior.
- **Alternatives considered**:
  - Retrying all 5xx responses (rejected: widens behavior beyond the
    specification and can duplicate non-idempotent work).
  - Unbounded retries (rejected: FR-012 caps the budget).
  - Sleeping without a context select (rejected: FR-013).
- **Known divergence**: `openaicompat.RetryDelay` is a third
  implementation of this policy alongside `vertexRetryDelay`
  (embed/vertex.go) and `vertexSynthRetryDelay` (llm/vertex.go). The
  duplication is intentional: the Vertex implementations cap the
  delay before applying jitter (so a jittered delay can exceed sixty
  seconds), while `RetryDelay` caps after jitter, and delegating
  either implementation to the other would change observable Vertex
  behavior (backward compatibility). Consolidating the three into one
  shared helper is left as follow-up work.

## R8: Error hygiene and credential safety

- **Decision**: Prefix every failure with
  `openai-compatible <capability>`, include the HTTP status, and append
  a response excerpt capped at 512 bytes with every occurrence of the
  resolved key replaced by `[REDACTED]`. Never log request headers or
  bodies. Rely on `net/http`'s default redirect policy, which strips
  `Authorization` when the destination host differs from the original.
- **Rationale**: FR-019 and FR-020 forbid credential material in logs,
  errors, and artifacts, and require a bounded excerpt. The standard
  library already implements safe cross-host redirect handling, so a
  custom policy would add risk without benefit.
- **Alternatives considered**:
  - A custom `CheckRedirect` that rejects all redirects (rejected:
    breaks legitimate gateway redirects; the spec allows following
    them without forwarding credentials).
  - Logging request bodies for debugging (rejected: user content and
    potential secrets).

## R9: Diagnostics integration

- **Decision**: Add `openai-compatible` branches to the doctor's
  embedding and synthesis layers that report configuration
  completeness and probe status without secrets, and include the new
  value in the unknown-provider message. Update startup, doctor, and
  shared indexing-helper warnings (`createIndexEmbedder`, reached by
  `dewey index`, `reindex`, `compile`, `curate`, and `lint`) so the
  `ollama pull` hint appears only for Ollama. Surface the unavailable
  reason in semantic, compile, and curate tool errors.
- **Rationale**: A valid gateway configuration reported as "unknown
  provider (supported: ollama, vertex)" would violate Observable
  Quality and mislead operators. FR-015 requires an actionable error
  for semantic tools, and the current message names Ollama only.
- **Alternatives considered**:
  - Leave diagnostics untouched (rejected: a valid configuration would
    show a FAIL line).
  - Rewrite the whole doctor layer (rejected: out of scope; Ollama and
    Vertex output must stay unchanged).

## R10: Testing and quality gates

- **Decision**: Use `httptest` stub gateways for every contract test;
  no live service, no credentials, no network. Include race tests for
  concurrent `Available()` plus requests. Replicate the exact CI
  commands from `.github/workflows/ci.yml`, including the current
  Gaze thresholds `--max-crapload=48` and `--max-gaze-crapload=45`.
- **Rationale**: Constitution IV requires isolated tests, and MP-023
  requires commands derived from CI rather than memory. The AGENTS.md
  example thresholds are stale relative to the workflow file; the
  stale example itself is scheduled for correction in task T040.
- **Alternatives considered**:
  - A live-gateway integration test (rejected: nondeterministic,
    requires credentials, and violates Testability).
  - Copying thresholds from AGENTS.md (rejected: the workflow file is
    the source of truth).

## R11: Documentation and release surface

- **Decision**: Update README provider configuration and environment
  variable tables, update AGENTS.md provider configuration and package
  layout, add GoDoc to every new exported symbol, and file a website
  documentation sync issue in `unbound-force/website`. Blog and
  tutorial coverage opportunities are recorded as tasks T045 and
  T046, to be filed at implementation time rather than before the
  feature exists.
- **Rationale**: The constitution's Documentation Gate and Website
  Documentation Sync rule require both. New configuration fields and
  environment variables are user-facing behavior.
- **Alternatives considered**:
  - Code-only change (rejected: violates the Documentation Gate).
  - Deferring the website issue (rejected: the constitution requires
    it when user-facing behavior changes).

## R12: Capability-scoped HTTP timeouts

- **Decision**: The shared client uses a 30-second timeout for
  embedding requests and a 300-second timeout for synthesis requests,
  matching the Vertex providers (`embed/vertex.go` uses 30 seconds;
  `llm/vertex.go` `vertexSynthTimeout` is 300 seconds to allow large
  curation prompts to complete). The availability probe keeps its own
  one-second timeout (R3).
- **Rationale**: The issue requires behavior consistent with Vertex.
  This knowingly diverges from the specs/013-knowledge-compile
  llm-interface contract's 120-second synthesis invariant, which
  governs the Ollama synthesizer; spec 016's Vertex synthesizer
  already made the same 300-second choice with a recorded code
  comment, and this decision records the divergence explicitly rather
  than leaving it silent. FR-025 anchors the values and T013 verifies
  them.
- **Alternatives considered**:
  - 120 seconds to match the 013 Ollama contract (rejected: risks
    truncating large curation prompts through slow gateways; Vertex
    already established 300 seconds).
  - No client timeout (rejected: a gateway that accepts the TCP
    connection but never responds hangs `dewey index` or `dewey
    compile` indefinitely while passing every other check).

## R13: llm package imports and the 013 leaf-dependency invariant

- **Decision**: Proceed with `llm → openaicompat` alongside the
  pre-existing `llm → embed` imports in `llm/provider.go` and
  `llm/config.go` (present since spec 016's provider-config
  consolidation). Record the divergence rather than restructure.
- **Rationale**: The specs/013-knowledge-compile llm-interface
  contract's invariant 7 says `llm` MUST NOT import any Dewey
  package (leaf dependency). Spec 016 already broke that invariant
  silently when it consolidated provider configuration; 017 extends
  the same pattern with `llm → openaicompat`. Restructuring now —
  for example, extracting shared provider-config parsing into a
  fourth package — would widen this change beyond its scope. The
  divergence is recorded here and in plan.md's Structure Decision,
  mirroring the R12 discipline for the timeout divergence.
  `openaicompat` itself remains a true leaf: standard library plus
  `charmbracelet/log`.
- **Alternatives considered**:
  - Marking the 013 invariant superseded inside the 013 spec
    (rejected: editing a completed spec from another feature's
    branch; recording the divergence here is the lighter touch).
  - Extracting a shared config package so `llm` imports only it
    (rejected: still an import of a Dewey package, so the invariant
    is not restored).
