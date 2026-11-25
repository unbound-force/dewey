---
tag: hermetic-testing
author: jay-flowers
category: gotcha
created_at: 2026-11-19T00:24:19Z
identity: hermetic-testing-20261119T002419-jay-flowers
tier: draft
as_of: "dewey@50e8ff8ff6df4f2334c0dd2d67b92d99b27f7c2d"
---

Clearing endpoint env vars (DEWEY_EMBEDDING_ENDPOINT, OLLAMA_HOST) in a Go test helper does not make the command under test offline: embed.ReadEmbeddingConfig's fallback chain resolves to http://localhost:11434, so any command that embeds an availability probe (dewey status after the #225 fix, dewey doctor) still performs a real HTTP GET against the host when no endpoint is pinned. Tests that ignore the probe result still pass deterministically, but their network behavior is host-dependent (bounded by the probe's 5s context on pathological hosts). To make such tests fully hermetic, pin the fallback dead-end in the helper after clearing overrides — e.g. t.Setenv("OLLAMA_HOST", "127.0.0.1:1") — because per-vault config.yaml endpoints take precedence over the OLLAMA_HOST chain, so httptest-pinned tests are unaffected while everything else gets a deterministic connection-refused probe.
