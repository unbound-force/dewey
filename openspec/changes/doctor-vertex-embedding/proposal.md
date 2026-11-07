<!-- originating_issue: 143 -->

## Why

`dewey doctor`'s Embedding Layer section routes `provider: vertex`
through the Ollama diagnostics. A valid Vertex embedding setup is
reported as "ollama not running" / "model skipped (ollama not
reachable)" — or, with Ollama installed, gets an `ollama pull` fix
hint for a Vertex model name. Spec 017 left this gap deliberately
(FR-017, preserve Vertex behavior) and filed this issue.

## What Changes

- The Embedding Layer section branches on `embedding.provider`:
  `vertex` gets its own diagnostics.
- Vertex diagnostics report configuration completeness (model,
  project, region) through `embed.NewEmbedderFromConfig`, which makes
  no network calls — mirroring the Synthesis Layer's Vertex branch.
  Complete: `PASS config project=… region=… model=…`. Incomplete:
  `FAIL config incomplete: <missing field>`.
- No Ollama reachability check and no `ollama pull` hint for Vertex.
- The section header names the provider: `Embedding Layer (<model>
  via vertex)`.

Out of scope: Vertex runtime behavior for indexing or semantic
search; live credential checks.

## Capabilities

### New Capabilities
- None.

### Modified Capabilities
- `dewey doctor`: provider-aware Embedding Layer diagnostics for
  Vertex.

### Removed Capabilities
- None.

## Impact

- `cli.go` — new `printVertexEmbeddingChecks` helper; one switch case.
- `cli_test.go` — configured and misconfigured Vertex embedding.
- `README.md` — doctor section description updated for the
  provider-aware Embedding Layer.
- Website docs-sync issue in unbound-force/website for the doctor
  Embedding Layer output change (`docs` label, per the website#249
  precedent).
- No change for `ollama`, empty, or `openai-compatible` providers.

## Constitution Alignment

Assessed against the Dewey project constitution (v1.4.0).

### I. Composability First

**Assessment**: PASS

No live calls or credentials required; Ollama stays optional.

### II. Autonomous Collaboration

**Assessment**: N/A

Diagnostic output only.

### III. Observable Quality

**Assessment**: PASS

Diagnostics become accurate for the configured provider; fix hints
no longer mislead.

### IV. Testability

**Assessment**: PASS

Doctor output is asserted in isolated temp vaults.
