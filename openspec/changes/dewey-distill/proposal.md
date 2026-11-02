# Proposal: dewey-distill

## Why

Dewey's knowledge lifecycle already exists as MCP tools — `lint`,
`compile`, `curate`, and `promote` — but the server never wires a
synthesizer into `compile` or `curate`: `server.go` passes `nil` for
both, so every MCP call returns prompt mode even when a synthesis
provider is configured. An agent that wants compiled articles or
curated store content must therefore fall back to the CLI, and the
CLI cannot run while the MCP server is up because the server holds an
exclusive `flock` on `.uf/dewey/dewey.lock` (`store/store.go`). The
result is a dead end: the running server can read the knowledge state
but cannot advance it, and the process that can advance it cannot
start.

The same dead end blocks the human review gate. `promote` accepts a
single page and only resolves exact page names. Compiled articles
stored as `compiled/{tag}` are reported as "Page not found" when the
caller passes the bare tag (issue #91), and the error text masks real
store failures. There is no batch promotion, so approving twenty
learnings takes twenty tool calls.

This change closes the loop with a server-side, OpenAI-compatible
synthesizer and a `/dewey-distill` slash command that runs
lint → compile → curate → promote entirely through the running MCP
server, with mandatory human gates before synthesis scope and before
promotion.

The selected provider is **enmaas** — an OpenAI-compatible API at
`https://api.enmaas.devshift.net/v1` using model
`rits/zai-org/glm-5-3`. The API key is supplied by
`DEWEY_SYNTHESIS_API_KEY` or `synthesis.api_key` in `config.yaml`; it
is never committed and never logged.

Fixes GitHub issue #133 and subsumes issue #91 (compiled-page
promotion).

## What Changes

- **New `openai-compatible` synthesis provider** (`llm/`):
  `POST {endpoint}/chat/completions` with a bearer token, a
  configurable model, timeout, and max output tokens. HTTP 429 is
  retried with the same bounded exponential backoff and `Retry-After`
  handling as the Vertex provider (issue #77), and a response with
  `finish_reason: "length"` fails loudly instead of returning silently
  truncated text (issue #76). Configuration is added to the
  `synthesis:` block: `provider: openai-compatible`, `endpoint`,
  `model`, optional `api_key`, `timeout`, `max_tokens`. The API key
  resolves from `DEWEY_SYNTHESIS_API_KEY` first, then
  `synthesis.api_key`; it is redacted from error bodies and never
  logged.
- **Server-side synthesis wiring**: `executeServe` builds one
  synthesizer from `llm.ReadSynthesisConfig` and passes it into the
  server through a new `WithSynthesizer` option. `compile` and
  `curate` receive it instead of `nil`, and background curation
  reuses the same instance. When no synthesis model is configured, or
  the provider cannot be constructed, the option stays nil and both
  tools keep returning prompt mode byte-compatibly.
- **Shared index mutex for compile**: `compile` joins `curate` on the
  shared `indexMutex` so a compile cannot race a background index. The
  existing "already in progress" behavior is preserved, now keyed on
  the pipeline-wide lock.
- **Batch promotion and robust page resolution**: `promote` keeps
  `page` for single-shot compatibility and adds `pages []string`.
  Batch promotion validates every requested page before mutating any,
  returns per-page errors, and fails closed (no partial promotion).
  Page resolution becomes case-insensitive, accepts bare tags against
  the `learning/` and `compiled/` namespaces and `knowledge/*`
  suffixes, and matches `source_id`/`source_doc_id`. Real store
  errors are returned instead of being masked as "Page not found".
- **`/dewey-distill` slash command** (`.opencode/commands/dewey-distill.md`,
  registered in `slash_commands.go` and scaffolded by `dewey init`):
  lint → inventory → human gate on scope → compile/curate through
  MCP → human review gate → batch promote → report with a
  `/dewey-publish` suggestion. The command never shells out to the
  CLI, because the running server owns the store lock. In prompt mode
  it synthesizes compile prompts itself and calls `store_compiled`;
  curated content has no write-back tool, so it reports that
  server-side synthesis is required rather than fabricating output.
- **Doctor diagnostics**: `dewey doctor` validates the new provider's
  endpoint, model, and credential presence without a live API call and
  without printing the key; the unknown-provider failure text lists
  the new provider.
- **Documentation**: README provider configuration and CLI/promote
  sections, AGENTS.md provider/trust-tool notes, and GoDoc for every
  new exported symbol. A website sync issue is filed in
  `unbound-force/website`.

## Capabilities

### New Capabilities

- `knowledge-distillation`: Server-side synthesis for the existing
  lint/compile/curate/promote MCP lifecycle, including the
  OpenAI-compatible provider, prompt-mode fallback, batch promotion
  with namespace-aware page resolution, and the gated
  `/dewey-distill` workflow.

### Modified Capabilities

None. Existing MCP tool contracts for the 37 inherited graphthulhu
tools are untouched; `promote` gains an optional additive field.

### Removed Capabilities

None.

## Impact

- **`llm/`**: new `openai_compatible.go` (provider, request/response,
  429 retry, redaction) and tests; `ProviderConfig` gains `APIKey`,
  `Timeout`, `MaxTokens`; `ReadSynthesisConfig` reads the new YAML
  fields from per-vault and global config; `NewSynthesizerFromConfig`
  gains the `openai-compatible` branch and an updated supported-list
  error.
- **`tools/`**: `NewCompile` accepts the shared index mutex; `promote`
  gains batch input, namespace-aware resolution via `store.FindPage`,
  and truthful error propagation; tests for each.
- **`store/`**: new `FindPage` resolution helper with
  deterministic precedence and ambiguity detection; no schema change.
- **`server.go`**: `serverConfig.synth` plus `WithSynthesizer`; compile
  and curate receive the configured synthesizer.
- **`main.go`**: `executeServe` builds the synthesizer after backend
  resolution; `backgroundCuration` accepts the shared instance instead
  of constructing its own.
- **`cli.go`**: `dewey promote` accepts one or more page arguments
  through the same batch path; doctor handles the new provider.
  `dewey compile`/`curate` CLI behavior is unchanged.
- **`slash_commands.go` / `dewey init`**: new `dewey-distill.md` map
  entry, scaffolded with the existing non-overwriting mechanism; the
  command file is also tracked in `.opencode/commands/`.
- **No new module dependencies, no CGO, no schema migration.** The
  provider uses `net/http` only. enmaas provides synthesis only;
  embeddings remain Ollama or Vertex.
- **Behavior while the server runs**: the whole pipeline works against
  the live MCP server without a restart or a CLI process. On the next
  server start the new compiled/curated files are indexed normally.
- **Out of scope**: enmaas embeddings, new lint checks, curation
  scheduling changes, and `/dewey-configure` (#122).

## Constitution Alignment

Assessed against the project constitution
(`.specify/memory/constitution.md`, extending the Unbound Force org
constitution; the org constitution prevails on conflict).

### I. Composability First

**Assessment**: PASS

Dewey remains independently installable and usable with no synthesis
provider: `compile` and `curate` fall back to the existing prompt
mode, byte-compatible with today, and the new provider is opt-in via
`config.yaml` or environment. The slash command uses MCP tools and
degrades to prompt mode when server-side synthesis is unavailable. No
new module dependency, no CGO, and no coupling to another Unbound
Force tool.

### II. Autonomous Collaboration

**Assessment**: PASS

The pipeline advances entirely through MCP tool calls (`lint`,
`list_pages`, `compile`, `curate`, `promote`, `store_compiled`), so
the agent-facing surface is structured tool results rather than
out-of-band process control. Human judgment arrives at two explicit
gates in the command layer, not as runtime coupling. The CLI keeps
working independently.

### III. Observable Quality

**Assessment**: PASS

Every stage reports a machine-parseable status (`compiled` /
`prompts_ready`, `complete` / `prompt_ready`, per-page promotion
results), with provenance (model, sources, timestamps, tiers).
Promotion failures identify each page and the real underlying error.
`dewey doctor` reports provider configuration state without exposing
credentials. The API key never appears in logs, errors, or output.

### IV. Testability

**Assessment**: PASS

The provider is tested against `httptest` servers covering success,
authentication failure, malformed responses, truncation, and 429
retry with `Retry-After`; no network or real credential is required.
Server wiring is tested with fake synthesizers and in-memory SQLite.
Store resolution and batch promotion are tested against `:memory:`
databases, including ambiguity, case mismatch, bare tags, and
all-or-nothing failure. The slash command's map entry is verified
byte-identical to the tracked file.
