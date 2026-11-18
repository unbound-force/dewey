## Why

`dewey status --json` reports wrong answers for embedding health, so anything
that checks embedding readiness from the CLI (scripts, CI, humans debugging
"why is semantic search empty") gets misled. Two defects, both verified against
the current source and reproduced on a fresh `dewey init` vault:

1. **`embeddingAvailable` is always `false`.** `statusData.EmbeddingAvailable`
   (`cli.go:546`) is serialized at `cli.go:734` but never assigned anywhere in
   the codebase — the field is dead. A machine whose Ollama serves the
   configured model still reports `"embeddingAvailable": false`.

2. **`embeddingModel` carries its config comment.** `readEmbeddingModel`
   (`cli.go:609`) parses `config.yaml` line-by-line and returns everything
   after `model:`, so the template `dewey init` writes (`cli.go:285`) —
   `model: granite-embedding:30m  # Override with DEWEY_EMBEDDING_MODEL env
   var` — surfaces as
   `"granite-embedding:30m  # Override with DEWEY_EMBEDDING_MODEL env var"`.
   Every scaffolded vault has this comment, so every vault reports the
   polluted string. The parser also ignores `DEWEY_EMBEDDING_MODEL`, so status
   can disagree with the model `dewey serve` actually uses, and it can match
   the first `model:` line anywhere in the file rather than the one under
   `embedding:`.

Reproduced (fresh `dewey init` vault, current `main`):

```json
"embeddingAvailable": false,
"embeddingModel": "granite-embedding:30m  # Override with DEWEY_EMBEDDING_MODEL env var",
```

The same polluted model string also reaches the text output's `Model:` line
because both formats render the shared `statusData.EmbeddingModel`.

The MCP `health` tool already reports both fields correctly
(`server.go:672-673` uses `embedder.ModelID()` and `embedder.Available()`),
and `dewey doctor` already resolves embedding config the way serve does
(`cli.go:1578`, `embed.ReadEmbeddingConfig`). Only the `dewey status` path
lags behind.

Fixes unbound-force/dewey#225. Phase 1 of epic unbound-force/dewey#226 —
scoped to exactly this issue; other epic pieces (e.g. startup re-embedding
cost) are separate changes.

## What Changes

- `dewey status` resolves the embedding model through
  `embed.ReadEmbeddingConfig(deweyDir)` — the same precedence serve uses
  (`DEWEY_EMBEDDING_MODEL` env var over per-vault `config.yaml` over the
  global `~/.config/dewey/config.yaml`, YAML-parsed so trailing comments are
  stripped, defaulting to `granite-embedding:30m` when nothing is
  configured). This deletes the hand-rolled `readEmbeddingModel`.
- `dewey status` sets `embeddingAvailable` by constructing the configured
  embedder and calling its `Available()` method — the same probe the MCP
  `health` tool uses (`server.go:673`). The probe is diagnostic-only: status
  never auto-starts Ollama (matching `dewey doctor`'s `autoStart=false`
  stance) and relies on the embedders' built-in bounded probes (5s for
  Ollama/Vertex, 1s for OpenAI-compatible gateways), so an unreachable
  provider cannot hang the command.
- The `--json` output keeps the same keys with corrected values:
  `embeddingModel` (clean model id) and `embeddingAvailable` (real probe
  result). No keys are added, renamed, or removed. The text output's
  `Model:` line shows the same corrected model via the shared
  `statusData`.
- No store, schema, or `dewey serve` changes: `graph.db` is untouched, the
  learning file format is untouched, and serve/startup behavior is unchanged.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `cli-status`: `dewey status` reports the serve-resolved embedding model and
  a real availability probe in both `--json` and text output.

### Removed Capabilities

None.

## Impact

- **Files**: `cli.go` (status command data collection; `readEmbeddingModel`
  removed), `cli_test.go` (status tests extended). No other packages change —
  `embed/`, `server.go`, and the store are consumed as-is.
- **Behavior changes (deliberate, observable)**:
  - `embeddingModel` loses the trailing comment on every scaffolded vault and
    now honors `DEWEY_EMBEDDING_MODEL` and the global config fallback — it
    reports what serve would use, which is the point of the fix.
  - A vault whose `config.yaml` is missing entirely now reports
    `granite-embedding:30m` (the serve default) instead of `""`.
  - `embeddingAvailable` flips from always-`false` to the probe result.
- **Out of scope**: the `dewey init` config template keeps its comment (valid
  YAML; harmless once parsed properly), doctor's Embedding Layer, the MCP
  `health` tool, any re-embedding/startup-cost work from epic #226, and any
  schema change (none needed).

## Constitution Alignment

Assessed against the Unbound Force org constitution.

### I. Autonomous Collaboration

**Assessment**: PASS

The fix improves an artifact consumers already rely on: `dewey status --json`
is a machine-parseable contract between Dewey and scripts/agents. Correcting
`embeddingAvailable` and `embeddingModel` makes that artifact truthful without
introducing any new runtime coupling — status probes the same embedder
interface serve uses. No new communication channels or shared state.

### II. Composability First

**Assessment**: PASS

Dewey stays fully usable without Ollama or any embedding provider: the probe
is one bounded call inside an optional reporting path, an unreachable or
uninstalled provider reports `embeddingAvailable: false` (with the model still
resolved from config), and the command keeps working exactly as before
otherwise. No new dependencies; the change consumes the existing
`embed.ReadEmbeddingConfig` and `Embedder.Available()` surface.

### III. Observable Quality

**Assessment**: PASS

This change is entirely about observable quality: `dewey status --json` is the
machine-parseable health report, and both corrected fields carry provenance by
construction — the model now names the exact model serve would use, and the
availability flag reflects a real probe rather than a dead constant. The JSON
contract (same keys, corrected values) is spec'd with verifiable scenarios.

### IV. Testability

**Assessment**: PASS

Model resolution is pure configuration reading, testable against temp-dir
config files with `t.Setenv` (the `newGatewayDoctorVault` pattern already in
`cli_test.go`). The availability probe is testable hermetically by pointing
the resolved endpoint at an `httptest` server — a tags list containing the
model yields `true`, a closed server or missing model yields `false` — no real
Ollama or network access required, matching the doctor gateway tests.
