---
tag: agent-tool-boundaries
author: jay-flowers
category: pattern
created_at: 2026-11-11T19:18:42Z
identity: agent-tool-boundaries-20261111T191842-jay-flowers
tier: draft
---

V1 OpenCode agent frontmatter `tools:` maps accept glob keys (e.g. `"dewey_*": false`) resolved with last-matching-rule-wins ordering, so a deny-all-then-allow-N shape host-enforces an MCP tool allowlist that prompt text alone cannot (curator-tool-allowlist, dewey #192). Effective mechanical guarding of that shape needs four layers, all shipped in dewey's cli_test.go: (1) an order-preserving parse — yaml.v3 `yaml.Node` keeps mapping key order, which a plain Unmarshal into a map destroys, and the deny-before-allow ordering is the load-bearing invariant; (2) registry anchoring — every allow entry must resolve against the tool-registry source (registeredMCPToolNames parses server.go), otherwise a typo or server-side rename leaves frontmatter and prompt text agreeing on a stale name while the glob denies the real tool, silently dropping a capability with green CI; (3) set-equality parity between the frontmatter allow set and the prompt-text allowlist so neither list drifts in either direction; (4) doctored-document negative paths executed through a recording `testing.TB` double (Errorf/Fatalf recorded, Fatalf degraded to record-and-continue so fixtures must stay structurally parseable) proving the guards reject each drift scenario by execution rather than inspection. Derive the other-server deny set from the repository's tracked MCP config (opencode.json) fail-closed, so a newly configured server without a matching `<key>_*: false` entry fails the test instead of passing vacuously.
