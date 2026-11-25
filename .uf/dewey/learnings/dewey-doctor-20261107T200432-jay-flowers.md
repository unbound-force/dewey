---
tag: dewey-doctor
author: jay-flowers
category: gotcha
created_at: 2026-11-07T20:04:32Z
identity: dewey-doctor-20261107T200432-jay-flowers
tier: draft
---

In dewey doctor-output tests, the vault fixture must clear DEWEY_EMBEDDING_MODEL alongside DEWEY_EMBEDDING_ENDPOINT, DEWEY_EMBEDDING_MODEL, DEWEY_SYNTHESIS_ENDPOINT, DEWEY_GENERATION_MODEL, and OLLAMA_HOST (see newGatewayDoctorVault in cli_test.go, opsx/doctor-vertex-embedding). embed/config.go gives DEWEY_EMBEDDING_MODEL env-over-config precedence, so a developer machine with that variable set silently replaces the config.yaml model and breaks assertions on the rendered model name — the tests pass on machi
