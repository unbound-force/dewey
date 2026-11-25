---
tag: dewey-doctor
author: jay-flowers
category: gotcha
created_at: 2026-11-07T20:46:11Z
identity: dewey-doctor-20261107T204611-jay-flowers
tier: draft
---

In dewey, VertexEmbedder.Available() (embed/vertex.go) invokes the lazy tokenFn backed by google.FindDefaultCredentials, which performs credential discovery and can reach the GCE metadata server over the network. No-network diagnostic paths therefore MUST validate Vertex configuration through the provider factory (embed.NewEmbedderFromConfig) and MUST NOT call Available() — unlike the openai-compatible doctor branch, which legitimately probes /models over HTTP. A future extension adding a "provider ready" check to the doctor Vertex branch (mirroring the gateway branch shape) would silently break DVE-FR-001's no-network contract; the factory is the correct validation seam because it rejects empty project/region (embed/provider.go) and empty model (NewVertexEmbedder) without any I/O.
