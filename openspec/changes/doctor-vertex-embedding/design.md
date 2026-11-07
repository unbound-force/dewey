## Context

`runDoctorChecks` switches on `embCfg.Provider` with
`case "ollama", "", "vertex":` sharing the Ollama path. The Synthesis
Layer already has a dedicated `vertex` case that validates config via
the provider factory without live calls.

Constitution alignment is assessed in the proposal (I Composability
First PASS, II Autonomous Collaboration N/A, III Observable Quality
PASS, IV Testability PASS). This design reinforces it: D2 validates
through the provider factory with no network calls (IV — no external
services in tests), and the dedicated branch leaves Ollama
diagnostics untouched (I — Ollama stays optional).

## Goals / Non-Goals

### Goals
- Accurate Vertex embedding diagnostics, no Ollama hints.

### Non-Goals
- Live Vertex credential or reachability checks.
- Changing Ollama or openai-compatible diagnostics.

## Decisions

### D1 Dedicated helper, one switch case

`runDoctorChecks` is already large; the Vertex branch lives in
`printVertexEmbeddingChecks(c, w, cfg)` so the doctor function gains
a single case.

### D2 Validate through the factory

`embed.NewEmbedderFromConfig` checks project, region, and model and
only builds an HTTP client — no network. Its error text names the
missing field, matching the Synthesis Layer's `incomplete:` style.

## Risks / Trade-offs

- A complete config can still fail at runtime (credentials, quota).
  The doctor says configuration is complete, not that calls succeed —
  the same contract as the Synthesis Layer's Vertex branch.
