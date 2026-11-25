---
tag: dewey-doctor
author: jay-flowers
category: pattern
created_at: 2026-11-07T20:46:20Z
identity: dewey-doctor-20261107T204620-jay-flowers
tier: draft
---

Verifying a "no network calls" spec requirement (e.g., dewey doctor DVE-FR-001, Vertex embedding diagnostics) requires tracing the full provider-construction chain, not just the diagnostic call site: embed.NewEmbedderFromConfig (embed/provider.go) validates project/region with field-naming errors, and NewVertexEmbedder (embed/vertex.go) validates model and builds only a struct plus http.Client — google.FindDefaultCredentials is deferred behind tokenFn and runs only at predict time. Factory validation is therefore network-free by design while runtime embedding still authenticates. Reviewers confirming "configuration completeness without network calls" should read the constructor chain and verify credential/token resolution is lazy; also confirm every field named in the completeness contract fails closed (empty model errors in the constructor, so no PASS-with-empty-model is reachable).
