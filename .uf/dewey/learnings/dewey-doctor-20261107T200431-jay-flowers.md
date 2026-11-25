---
tag: dewey-doctor
author: jay-flowers
category: pattern
created_at: 2026-11-07T20:04:31Z
identity: dewey-doctor-20261107T200431-jay-flowers
tier: draft
---

Dewey doctor provider-diagnostics branch shape (verified in doctor-vertex-embedding, DVE-FR-001, issue #143): the Embedding Layer switch routes each embedding.provider to its own case; Vertex validates configuration completeness through embed.NewEmbedderFromConfig and mirrors the Synthesis Layer vertex branch — header `(<model> via vertex)`, FAIL rendered as `incomplete: <factory error>` (the factory error names the missing field, e.g. "vertex embedding provider requires project"), PASS rendered as `project
