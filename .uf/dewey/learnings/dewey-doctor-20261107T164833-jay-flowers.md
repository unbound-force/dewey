---
tag: dewey-doctor
author: jay-flowers
category: pattern
created_at: 2026-11-07T16:48:33Z
identity: dewey-doctor-20261107T164833-jay-flowers
tier: draft
---

dewey doctor provider diagnostics follow a two-tier pattern: Ollama-backed providers get live reachability and model checks via ensureOllama, while cloud providers (Vertex) are validated for configuration completeness only, through the provider factory (embed.NewEmbedderFromConfig / llm.NewSynthesizerFromConfig), which performs no network calls — construction builds an HTTP client and defers credential discovery to a lazily invoked tokenFn. Doctor must never call Available() on OAuth-backed providers because it requires real GCP credentials and violates the diagnostic-only contract (established in doctor-synthesis-diagnostics design D2 and extended to the embedding layer by doctor-vertex-embedding, issue #143). New provider diagnostics should mirror the Synthesis Layer's Vertex branch: report `PASS config project=… region=… model=…` on factory success, `FAIL config incomplete: <missing field>` naming the field from the factory error, and never emit Ollama fix hints for non-Ollama providers.
