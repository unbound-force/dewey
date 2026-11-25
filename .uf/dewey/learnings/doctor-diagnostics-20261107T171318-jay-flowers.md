---
tag: doctor-diagnostics
author: jay-flowers
category: pattern
created_at: 2026-11-07T17:13:18Z
identity: doctor-diagnostics-20261107T171318-jay-flowers
tier: draft
---

dewey doctor's provider-safe diagnostic pattern: validate provider configuration through the constructor-only factory path (embed.NewEmbedderFromConfig / llm.NewSynthesizerFromConfig) and never call Available() on the constructed provider. Constructor validation is offline-safe — it checks required fields and allocates an HTTP client without dialing — while Available() on Vertex providers invokes the lazy tokenFn (google.FindDefaultCredentials, which can attempt GCE metadata-server discovery) and on Ollama/gateway providers performs live HTTP probes. This pattern was applied consistently in doctor-synthesis-diagnostics (Synthesis Layer vertex branch) and doctor-vertex-embedding (printVertexEmbeddingChecks, cli.go): construct, report the error verbatim as "incomplete: <missing field>" FAIL, discard the instance. Any future doctor section for a new provider should follow the same constructor-only contract.
