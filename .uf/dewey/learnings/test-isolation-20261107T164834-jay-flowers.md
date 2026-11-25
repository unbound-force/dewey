---
tag: test-isolation
author: jay-flowers
category: gotcha
created_at: 2026-11-07T16:48:34Z
identity: test-isolation-20261107T164834-jay-flowers
tier: draft
---

Dewey doctor-config tests that pin embedding model strings must clear DEWEY_EMBEDDING_MODEL: embed.ReadEmbeddingConfig applies it as a model override read at runDoctorChecks time, while the newGatewayDoctorVault helper clears only DEWEY_EMBEDDING_ENDPOINT, DEWEY_SYNTHESIS_ENDPOINT, DEWEY_GENERATION_MODEL, and OLLAMA_HOST. An exported DEWEY_EMBEDDING_MODEL in the environment produces false failures in TestDoctorCmd_EmbeddingVertexConfigured and TestDoctorCmd_EmbeddingGatewayConfigured even though the per-vault config.yaml is the intended source. When adding doctor tests that assert provider-specific config rendering, audit every env var that ReadEmbeddingConfig/ReadSynthesisConfig honor — not just the endpoint chain — and extend the shared helper rather than clearing ad hoc in each test.
