---
tag: test-isolation
author: jay-flowers
category: gotcha
created_at: 2026-11-07T17:13:33Z
identity: test-isolation-20261107T171333-jay-flowers
tier: draft
---

Dewey doctor test fixtures must clear every environment variable in the embedding/synthesis config resolution chain, not just the endpoint vars. embed.ReadEmbeddingConfig honors DEWEY_EMBEDDING_MODEL (embed/config.go) over config.yaml, so a developer machine with that variable set silently overrides the fixture's model and breaks value-specific assertions like "text-embedding-005 via vertex". newGatewayDoctorVault (cli_test.go) now clears DEWEY_EMBEDDING_ENDPOINT, DEWEY_EMBEDDING_MODEL, DEWEY_SYNTHESIS_ENDPOINT, DEWEY_GENERATION_MODEL, and OLLAMA_HOST, and points XDG_CONFIG_HOME at an empty t.TempDir() to isolate global config. Found in doctor-vertex-embedding round 1 (PR #158), fix verified in round 2. Any new doctor fixture that asserts resolved config values should clear the full chain up front.
