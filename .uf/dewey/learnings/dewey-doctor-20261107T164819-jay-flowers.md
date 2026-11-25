---
tag: dewey-doctor
author: jay-flowers
category: pattern
created_at: 2026-11-07T16:48:19Z
identity: dewey-doctor-20261107T164819-jay-flowers
tier: draft
---

Dewey doctor validates provider configuration through the provider factory (embed.NewEmbedderFromConfig / llm.NewSynthesizerFromConfig), which only constructs a client struct — no network I/O, no credential discovery (google.FindDefaultCredentials runs only at request time inside tokenFn). This makes the factory a safe offline config-completeness validator for diagnostics. When adding a doctor branch for a new provider: (1) validate via the factory, never via live probes, (2) never route a non-Ollama provider through Ollama reachability checks or `ollama pull` hints (the issue #143 bug class — a Vertex config got "ollama pull text-embedding-005" fix hints), and (3) mirror the check-label semantics honestly ("config" PASS means complete configuration, not reachable — the Synthesis Layer Vertex branch contract). Doctor tests stay hermetic by reusing newGatewayDoctorVault (t.TempDir + XDG_CONFIG_HOME isolation + cleared OLLAMA_HOST/endpoint env vars), so assertions never depend on a locally running Ollama.
