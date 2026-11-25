---
tag: hermetic-testing
author: jay-flowers
category: pattern
created_at: 2026-11-18T23:47:11Z
identity: hermetic-testing-20261118T234711-jay-flowers
tier: draft
as_of: "dewey@af81010d2e226f1b1f3229c019aae4cad19004e2"
evidence:
  - "dewey:cli_test.go"
  - "dewey:embed/config.go"
  - "dewey:embed/embed.go"
---

Hermetic Go tests for CLI commands that resolve config and probe providers need no injected production seam when the config precedence chain is the seam. Pattern (from dewey's status embedding tests, cli_test.go newStatusTestVault): pin XDG_CONFIG_HOME at a temp dir — embed.globalConfigDir() reads $XDG_CONFIG_HOME directly (embed/config.go:58), so this masks the developer's real global config on every platform (unlike os.UserConfigDir, which ignores XDG on macOS); clear DEWEY_EMBEDDING_ENDPOINT / DEWEY_EMBEDDING_MODEL / OLLAMA_HOST by t.Setenv to empty strings (the embed chain treats empty as unset for all three, embed/config.go:31,42,155); steer availability probes at httptest servers via vault config.yaml or the DEWEY_EMBEDDING_ENDPOINT override. Two gotchas: (1) DEWEY_EMBEDDING_ENDPOINT overrides config.yaml, so a shared vault fixture must not pin it unconditionally — tests exercising config-resolved endpoints need it cleared, and endpoint pins belong in the specific assertion paths that ignore the probe; (2) tests that discard the probe result (model-resolution assertions) still execute it against the resolved default endpoint (localhost:11434), bounded at 5s by checkModelAvailable — pin the endpoint to a deterministically-refused address in those tests so the ignored probe fails fast instead of paying the timeout on firewalled hosts. (Submitted by divisor-testing during code review of opsx/dewey-status-json-says-embeddings-are-un, 2026-11-18.)
