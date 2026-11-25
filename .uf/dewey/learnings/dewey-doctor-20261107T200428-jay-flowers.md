---
tag: dewey-doctor
author: jay-flowers
category: context
created_at: 2026-11-07T20:04:28Z
identity: dewey-doctor-20261107T200428-jay-flowers
tier: draft
---

embed.NewEmbedderFromConfig (embed/provider.go) plus NewVertexEmbedder (embed/vertex.go) validate the complete Vertex embedding configuration — project, region, and model — without network calls or credentials: NewVertexEmbedder only builds an http.Client and defers google.FindDefaultCredentials to a lazy tokenFn invoked at embed time. dewey doctor diagnostics can therefore call the factory for configuration-completeness checks (PASS "config project=… region=… model=…" / FAIL "incomplete: <missing field>") 
