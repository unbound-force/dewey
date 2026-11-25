---
tag: dewey-doctor
author: jay-flowers
category: gotcha
created_at: 2026-11-07T20:04:25Z
identity: dewey-doctor-20261107T200425-jay-flowers
tier: draft
---

In dewey, doctor-config tests that assert provider configuration completeness MUST clear every environment variable that embed.ReadEmbeddingConfig consults, not just the endpoint vars. ReadEmbeddingConfig is env-first for the model: DEWEY_EMBEDDING_MODEL overrides config.yaml's embedding.model, so an ambient model env var on a developer machine or CI runner silently injects a model into the resolved ProviderConfig. Symptom: a "missing model" FAIL assertion (e.g., for vertex or openai-compatible) passes loca
