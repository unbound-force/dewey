---
tag: gate-integrity
author: jay-flowers
category: pattern
created_at: 2026-11-11T15:19:49Z
identity: gate-integrity-20261111T151949-jay-flowers
tier: draft
---

When a verification gate mirrors an external tool's configuration, enumerate the external tool's actual load paths — not the platform convention. dewey's `dewey gate auto-answer` resolved the opencode-auto-question plugin config XDG-first, while the plugin always reads `join(homedir(), ".config", "opencode", "auto-question.jsonc")` and never consults XDG_CONFIG_HOME, so a divergent XDG_CONFIG_HOME made the gate verify a file never read while the real config went unchecked — the gate failed open (dewey#191). The fix contract used in gate-config-path-parity: check every path the verified party could read (authoritative path always, secondary paths when they differ), refuse on any offending or unverifiable checked path, and name every checked file in the verdict on both pass and refusal. Fail-closed beats standardization: when you cannot change the verified party, mirror it.
