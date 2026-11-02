# Design: dewey-distill

## Context

`newServer` constructs `tools.NewCompile(..., nil, ...)` and
`tools.NewCurate(..., nil, ...)` (`server.go:130-136`), so the running
MCP server never performs synthesis even when `config.yaml` names a
provider. Both tools correctly return prompt mode for a nil or
unavailable synthesizer, and `backgroundCuration` (`main.go:797+`)
already demonstrates the intended construction sequence —
`llm.ReadSynthesisConfig` → `llm.NewSynthesizerFromConfig` →
`Available()` — for its own goroutine. The missing piece is wiring that
synthesizer into the request path.

The CLI cannot be the workaround: `store.Store` takes an exclusive
`Flock(LOCK_EX|LOCK_NB)` on `.uf/dewey/dewey.lock`
(`store/store.go:105-120`), so `dewey compile`, `dewey curate`,
`dewey lint`, and `dewey promote` all fail while the server runs.
The pipeline must be MCP-native.

`promote` is single-page and `store.GetPage` is an exact-name lookup.
Compiled pages are stored as `compiled/{tag}` by both `compile` and
`store_compiled`; callers who pass the bare tag get "Page not found"
(issue #91), and the tool returns the same text for a real query
failure, hiding the cause. A human review gate over many articles
needs batch promotion.

Constitution alignment (PASS x4) is assessed in the proposal. This
design reinforces it: the provider is independently usable (I),
delivered through MCP tool calls (II), reports status and provenance
without leaking secrets (III), and is exercised with `httptest` and
`:memory:` stores only (IV).

## Goals / Non-Goals

### Goals

- Server-side synthesis for `compile` and `curate` whenever a
  provider is configured and constructible, with no CLI process and no
  server restart.
- An `openai-compatible` synthesizer suitable for enmaas
  (`https://api.enmaas.devshift.net/v1`, model
  `rits/zai-org/glm-5-3`) with configurable timeout and output limit,
  bounded 429 retry, credential redaction, and truncation detection.
- Byte-compatible prompt mode when synthesis is unconfigured or
  unavailable.
- Batch promotion with namespace-aware resolution, all-or-nothing
  semantics, per-page errors, and truthful store errors (#91).
- A `/dewey-distill` command that drives lint → inventory → scope gate
  → compile/curate → review gate → batch promote → report.
- Doctor diagnostics for the new provider with no live call.

### Non-Goals

- enmaas embeddings. Embeddings stay Ollama/Vertex.
- New lint checks, curation scheduling, or `/dewey-configure` (#122).
- Changing the `Synthesizer` interface contract.
- Retrying non-429 provider errors.
- A rollback of already-promoted pages if a later page in the same
  batch fails — the batch validates first, so no partial write occurs.

## Decisions

### D1: Provider shape — one `openai-compatible` implementation

Add `llm/openai_compatible.go` implementing `Synthesizer`. Requests are
`POST {endpoint}/chat/completions` with
`{"model", "messages":[{"role":"user","content":prompt}], "max_tokens"}`
and `Authorization: Bearer <key>`. Responses are parsed from
`choices[0].message.content`. The endpoint may be a base URL
(`.../v1`) or already end in `/chat/completions`; both are accepted.

Rejected: an enmaas-specific type. The wire format is the
OpenAI-compatible contract, so one implementation serves enmaas and
any compatible gateway; provider naming is `openai-compatible`.

### D2: Configuration and credential precedence

`ProviderConfig` gains `APIKey`, `Timeout`, and `MaxTokens`. The
`configFile.Synthesis` struct gains matching YAML fields:

```yaml
synthesis:
  provider: openai-compatible
  endpoint: https://api.enmaas.devshift.net/v1
  model: rits/zai-org/glm-5-3
  timeout: 300s        # optional, default 300s
  max_tokens: 16000    # optional, default 16000
  # api_key: prefer DEWEY_SYNTHESIS_API_KEY instead of committing a key
```

The API key resolves `DEWEY_SYNTHESIS_API_KEY` first, then
`synthesis.api_key`. The environment variable wins so operators can
inject a secret without editing a file. `yaml.v3` decodes
`time.Duration` from a string such as `300s`; zero values fall back to
the defaults. Existing per-vault and global precedence in
`ReadSynthesisConfig` is preserved.

### D3: Availability is configuration completeness, not a network probe

`OpenAICompatibleSynthesizer.Available()` returns true when endpoint,
model, and key are all non-empty; it performs no network call. This
matches Vertex's credential-presence check, avoids startup latency and
flaky readiness decisions, and lets `Synthesize` surface the real HTTP
error (401/404/429) to the caller. The server still builds the
synthesizer only when a model is configured and the factory accepts the
config; if those checks fail the option is nil and tools return prompt
mode.

### D4: Bounded retry and truncation guard

Mirror the Vertex constants: 300s timeout, 16000 max tokens, 5
retries, 1s base delay, 60s cap, exponential backoff with ±25% jitter,
`Retry-After` integer seconds honored, context cancellation checked
between retries. Only HTTP 429 is retried. A `finish_reason` of
`length` returns an error naming `synthesis.max_tokens` instead of
silently returning truncated content (issue #76). Response bodies are
read with a 50 MiB limit and truncated to 512 bytes in error messages;
the key is replaced with `[REDACTED]` in any body text before it is
returned or logged (issue #77 covers adequate limits).

Rejected: retrying 500/502/503. It would amplify load during provider
outages and diverge from the Vertex contract.

### D5: One synthesizer instance, shared by tools and background curation

`serverConfig` gains `synth llm.Synthesizer` with a `WithSynthesizer`
option. `executeServe` resolves the vault path from the already-built
server options, calls `llm.ReadSynthesisConfig` and
`llm.NewSynthesizerFromConfig` once, and appends `WithSynthesizer` when
construction succeeds. `newServer` passes that instance to
`tools.NewCompile` and `tools.NewCurate`. `backgroundCuration` takes
the same instance as a parameter and keeps its own `Available()` check
after indexing completes, preserving the late-start case where Ollama
comes up during indexing. On any construction failure the instance is
nil and both tools keep prompt mode.

Rejected: constructing a synthesizer per tool call. It would repeat
availability checks and make the request path depend on mutable config.

### D6: Compile joins the shared index mutex

`tools.NewCompile` takes `mu *sync.Mutex` (nil creates a private one)
in the same position as `NewCurate`, and `Compile` uses
`TryLock`/`Unlock` on it. `server.go` and `cli.go` pass the shared
`indexMutex`/a fresh mutex respectively. This prevents a compile from
reading learnings while a background index rewrites pages, without
serializing compile against unrelated requests. The error text becomes
the index-pipeline message ("compilation cannot run while indexing is
in progress") and still refuses concurrent compiles.

Rejected: keeping a private compile mutex and adding a second shared
lock. Two locks invite lock-order bugs and still allow the index
race.

### D7: Promote resolution ladder and all-or-nothing batch

Add `store.FindPage(name)` with deterministic tiers:

1. exact `name = ?`;
2. `name = ? COLLATE NOCASE`;
3. for bare names, `compiled/{name}` and `learning/{name}` (NOCASE),
   then any `knowledge/%/{name}` (NOCASE);
4. for namespaced input, `source_id = <namespace>` and
   `source_doc_id = <rest>` (NOCASE); for bare input, the same pair
   across `compiled` and `learning`.

The first tier with exactly one match wins; more than one match is an
ambiguity error naming the candidates. Empty input is a validation
error. Query failures propagate with context.

`PromoteInput` adds `Pages []string` while keeping `Page`. When only
`Page` is set the result shape is unchanged. When `Pages` is set (and
`Page` is empty; setting both is a validation error), the tool first
resolves and validates every page (exists, tier `draft`), and only if
all pass does it update tiers. Any failure returns an error result
with a `failed` array of `{page, error}` entries and mutates nothing.
Success returns `{status: "promoted", promoted: [...], total_promoted,
message}`. Duplicate names are deduplicated in request order. The CLI
accepts one or more arguments and routes through the same path.

Rejected: partial promotion with a warning. A human gate that approves
a set should either apply the set or explain why it cannot, and partial
writes would leave the review gate's decision only half-applied.

### D8: `/dewey-distill` is MCP-only and gate-driven

The command (`.opencode/commands/dewey-distill.md`, also a
`deweySlashCommands` entry so `dewey init` scaffolds it) runs:

1. `lint` (fix=false) and presents findings.
2. `list_pages` for `learning` and `compiled` namespaces; presents
   counts and names, noting tiers.
3. Mandatory human gate: which tags to compile (full or incremental),
   and which knowledge stores to curate (or skip curate).
4. `compile` then `curate` through MCP. If `compile` returns
   `prompts_ready`, the command synthesizes each returned prompt and
   calls `store_compiled` per cluster. If `curate` returns
   `prompt_ready`, it reports that curated content requires
   server-side synthesis (there is no store-write MCP tool for
   curation) and leaves stores unchanged.
5. Mandatory human review gate: which compiled articles and learnings
   to promote.
6. A single `promote` call with `pages`. If the batch fails, relay
   the per-page errors and do not retry destructively.
7. Report: lint delta, articles written, curated results, promoted
   pages, and a suggestion to run `/dewey-publish`.

The command must not run `dewey` CLI subcommands that open the store,
because the server holds the lock.

Rejected: a single-click "do everything" command. Promotion is a trust
boundary; the command keeps the two gates explicit rather than
delegating judgment to the model.

### D9: Doctor validates without live calls

`dewey doctor` gains `case "openai-compatible"`: it prints the section
with endpoint and model, reports PASS when endpoint/model/key are
present, and FAIL with an actionable message when a field is missing
(naming `DEWEY_SYNTHESIS_API_KEY` or `synthesis.api_key`). It never
prints the key and makes no HTTP request. The default branch's
supported-provider list includes the new provider.

### D10: Backward compatibility

The `Synthesizer` interface is unchanged. Existing `ReadSynthesisConfig`
precedence is unchanged except for the new optional fields. Existing
configurations without `api_key`/`timeout`/`max_tokens` parse exactly
as before. `promote` with `page` returns the existing result shape.
Compile/curate prompt-mode payloads are untouched. Tool counts and the
37 inherited graphthulhu tools are unaffected; `promote` gains an
optional input field only.

## Risks

- **Provider variance**: OpenAI-compatible vendors differ in error
  bodies and `finish_reason` values. The implementation tolerates a
  missing/unknown `finish_reason` (only `length` is special), treats a
  missing `content` as an error, and surfaces the HTTP status.
- **Key leakage**: request headers are never logged, and response
  bodies are scrubbed of the key before they appear in errors. Tests
  assert the key is absent from all error strings.
- **Batch atomicity**: validation-first means one already-validated
  page refuses the whole batch. The command filters to `draft` pages
  before calling and shows the failure list; this is the intended
  fail-closed behavior.
- **Startup cost**: with `Available()` as a local check, enabling
  synthesis adds no network call at server start.
