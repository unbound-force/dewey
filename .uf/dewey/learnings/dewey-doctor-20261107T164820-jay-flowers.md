---
tag: dewey-doctor
author: jay-flowers
category: pattern
created_at: 2026-11-07T16:48:20Z
identity: dewey-doctor-20261107T164820-jay-flowers
tier: draft
---

dewey doctor provider branches follow a fixed shape: validate configuration through the provider factory (embed.NewEmbedderFromConfig / llm.NewSynthesizerFromConfig), which makes no network calls and whose error text names the missing field ("vertex embedding provider requires project"); render PASS "project=… region=… model=…" or FAIL "incomplete: <err>". Vertex branches never get Ollama reachability checks or `ollama pull` hints. The Synthesis Layer vertex branch in cli.go is the canonical shape; printVertexEmbeddingChecks (issue #143) mirrors it for the Embedding Layer.
