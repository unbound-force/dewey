## Context

`dewey status` collects data in `queryStoreStatus` (`cli.go`) and renders it
through `formatStatusText` / `formatStatusJSON`. Two embedding fields are
broken (issue #225, verified against current `main` and reproduced on a fresh
`dewey init` vault):

- `statusData.EmbeddingAvailable` (`cli.go:546`) is serialized at `cli.go:734`
  but never assigned anywhere — always `false`.
- `EmbeddingModel` comes from `readEmbeddingModel` (`cli.go:609`), a line
  parser that returns everything after `model:`, so the trailing comment in
  the config template `dewey init` writes (`cli.go:285`) reaches the output,
  and `DEWEY_EMBEDDING_MODEL` / the global config are ignored.

The correct patterns already exist in-repo and are reused verbatim by this
design:

- **Serve** resolves config via `embed.ReadEmbeddingConfig(deweyDir)`
  (`main.go:635`) and probes via `embedder.Available()`.
- **The MCP `health` tool** reports `cfg.embedder.ModelID()` and
  `cfg.embedder.Available()` (`server.go:672-673`).
- **`dewey doctor`** already renders an Embedding Layer from
  `embed.ReadEmbeddingConfig` with a diagnostic-only probe
  (`cli.go:1577-1645`), including the explicit stance that diagnostic
  commands never auto-start Ollama (`ensureOllama(..., false, nil)`).

The `Available()` implementations are internally time-bounded: Ollama's
`checkModelAvailable` uses a 5s context (`embed/embed.go:267`), Vertex a 5s
context (`embed/vertex.go:290`), and the OpenAI-compatible client a 1s
`/models` probe (`openaicompat/client.go` `probeTimeout`). No new timeout
machinery is needed.

## Goals / Non-Goals

### Goals

- `dewey status --json` reports `embeddingModel` exactly as serve would
  resolve it and `embeddingAvailable` as a real probe result (STA-FR-001,
  STA-FR-002).
- The text output's `Model:` line shows the same corrected model (it renders
  the shared `statusData.EmbeddingModel`).
- The `--json` key set is unchanged (STA-FR-003).
- Hermetic tests with no dependency on a running Ollama (constitution IV).

### Non-Goals

- Changing the `dewey init` config template. The trailing comment is valid
  YAML; once parsed as YAML it is harmless. Existing vaults keep their
  comment either way.
- Changing `dewey doctor`, the MCP `health` tool, or any serve/index/reindex
  path — they already behave correctly.
- Adding new text-output lines (the speckit 001 contract's illustrative
  `Available:`/`Dimension:` text block stays unimplemented).
- Any `graph.db` schema change, learning-format change, or epic #226
  startup/re-embedding work.

## Decisions

### D1: Resolve the model through `embed.ReadEmbeddingConfig`, delete `readEmbeddingModel`

`queryEmbeddingStatus` (new, below) calls `embed.ReadEmbeddingConfig(deweyDir)`
and uses `cfg.Model`. This is the single source of truth serve and doctor
already use, so status can no longer disagree with them. It fixes all four
defects of the line parser at once: trailing comments (YAML parse),
`DEWEY_EMBEDDING_MODEL` precedence, the global-config fallback, and matching
the first `model:` line anywhere in the file rather than the one under
`embedding:`.

Rejected alternatives:

- *Strip comments inside the line parser* — still wrong for env-var
  precedence, global config, and cross-section `model:` collisions; a second
  config parser would exist only to diverge from the first.
- *Remove the comment from the init template* — existing vaults keep theirs,
  and the pollution is a parsing bug, not a template bug.

The old parser's stated reason ("avoid a YAML dependency for status display")
is moot: the `embed` package already imports `gopkg.in/yaml.v3`, and package
`main` already imports `embed`.

### D2: Probe availability via `NewEmbedderFromConfig` + `Available()`, diagnostic-only

`queryEmbeddingStatus` constructs the configured embedder with
`embed.NewEmbedderFromConfig(cfg)` and reports `embedder.Available()`, the
same probe the `health` tool uses. Two boundaries:

- **No auto-start.** Status never calls `ensureOllama`. Doctor's comment
  ("diagnostic-only, it should never start subprocesses", `cli.go:1586`)
  states the rule this change follows: a reporting command must not spawn
  daemons as a side effect. `Available()` returning `false` already covers
  "not running".
- **Construction failure is `false`, not an error.** A misconfigured provider
  (e.g. Vertex without `project`) yields `embeddingAvailable: false` while
  the model still reports from config; the command succeeds. Doctor renders
  a FAIL check for this case; status has no check concept, only the boolean.

Timeout: rely on the embedders' built-in bounded probes (5s Ollama/Vertex,
1s gateway). Worst case adds ~5s to `dewey status` when the provider is
unreachable — the same bound `dewey doctor` already accepts. A wrapper would
add nothing: `Available()` takes no context, so a goroutine+select wrapper
could not cancel the underlying request and would only mask its result.

### D3: The probe lives in a sibling helper, not in `queryStoreStatus`

New helper in `cli.go`, placed and called like the existing trust helper:

```go
// queryEmbeddingStatus resolves the embedding model the way serve does and
// probes provider availability. Diagnostic-only: never starts processes.
func queryEmbeddingStatus(deweyDir string) (model string, available bool)
```

Called from the status `RunE` as
`data.EmbeddingModel, data.EmbeddingAvailable = queryEmbeddingStatus(deweyDir)`,
after `queryStoreStatus` and alongside `data.Trust = queryTrustStatus(vp)`.
The `EmbeddingModel` initialization moves out of `queryStoreStatus`'s struct
literal so model and availability always come from one resolved config.

Rationale: `queryStoreStatus` stays store-only (single responsibility), so
its tests remain hermetic without env pinning, and the network probe is
isolated in a function whose tests pin the endpoint deliberately. A two-value
return beats a Result struct here — with two fields the struct is ceremony
(the pack's Options/Result pattern earns its keep at three or more).

### D4: Tests pin the endpoint instead of injecting fakes

No new production seam is introduced. The existing configuration precedence
makes the probe hermetic: tests write a `config.yaml` (or set
`DEWEY_EMBEDDING_ENDPOINT`) pointing at an `httptest` server that serves an
Ollama `/api/tags` body listing (or omitting) the configured model. This is
exactly the established pattern — `newGatewayDoctorVault`
(`cli_test.go:7098`) builds a temp vault with a config and clears
`XDG_CONFIG_HOME`, `DEWEY_EMBEDDING_MODEL`, `DEWEY_EMBEDDING_ENDPOINT`, and
`OLLAMA_HOST` so ambient state cannot leak in; the doctor gateway tests
(`TestDoctorCmd_EmbeddingGatewayConfigured`) drive `Available()` through an
`httptest` server the same way.

Existing status tests (`TestStatusCmd_TextOutput`,
`TestStatusCmd_JSONOutput`) assert neither embedding field, so they stay
green regardless of whether the host running them has Ollama.

## Risks / Trade-offs

- **Status gains a network probe (bounded).** A vault pointing at an
  unreachable endpoint adds up to ~5s before the report prints. Accepted:
  identical to doctor's existing behavior, and the alternative (reporting a
  hardcoded `false`) is the bug being fixed.
- **Deliberate observable changes** (recorded in the proposal and spec):
  missing `config.yaml` now reports `granite-embedding:30m` instead of `""`;
  the global config and `DEWEY_EMBEDDING_MODEL` are now honored. These are
  the serve-parity behaviors the issue requests.
- **`Available()` has no timeout contract at the interface level.** A future
  embedder with an unbounded probe could hang status. Accepted: all three
  current implementations are bounded, and the right fix for a future
  offender is a context-aware interface evolution, not a wrapper here.
- **Fresh-embedder cache is cold each run** — `OllamaEmbedder` caches
  availability for 30s per instance; status constructs a new embedder per
  invocation, so no stale results are possible (and none are carried across
  runs).

## Constitution Alignment

This design applies the alignment claimed in `proposal.md`:

- **Observable Quality (III)** is the change's substance — the
  machine-parseable contract (`embeddingModel`, `embeddingAvailable`) becomes
  truthful, with values that carry their own provenance (the serve-resolved
  model; a real probe).
- **Testability (IV)** drives D3/D4: the probe is isolated in one helper and
  made hermetic through configuration precedence plus `httptest`, no
  external services required.
- **Composability First (II)** bounds D2: no new dependencies, and a machine
  without any embedding provider keeps a fully working `dewey status` that
  reports `false`.
- **Autonomous Collaboration (I)** is preserved by not touching the artifact
  boundaries: same CLI surface, same JSON keys, no new coupling between
  components.
