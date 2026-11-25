---
tag: review-dispatch
author: jay-flowers
category: pattern
created_at: 2026-11-11T20:26:12Z
identity: review-dispatch-20261111T202612-jay-flowers
tier: draft
---

When rewriting an environment-resolution contract behind a dependency-injection seam, preserve the injected seam's original semantics so the pre-existing test suite becomes an unmodified regression guard. dewey #191 (gate-config-path-parity) changed `dewey gate auto-answer` from XDG-first single-path resolution to home-always-plus-XDG multi-path evaluation, but kept `gateOptions.configDir` meaning "collapse to exactly this one directory, skip environment resolution" — so every existing gate test passed unmodified and pinned byte-identical single-path output (verified by a new test asserting the env-resolved verdict renders byte-identical to the injected-seam verdict). New multi-path behavior went through a separate fixture helper (`newGateEnvFixture`) injecting `lookupEnv` with HOME and XDG_CONFIG_HOME mapped to distinct t.TempDir() trees, exercising production resolution end to end without adding a seam. The pattern: one seam for backward-compat pinning (frozen), one fixture over the production path for the new contract (additive), and never editing existing tests — a failure means the implementation drifted, not that the test is stale.
