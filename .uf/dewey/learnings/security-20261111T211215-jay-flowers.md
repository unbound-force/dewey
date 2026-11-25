---
tag: security
author: jay-flowers
category: pattern
created_at: 2026-11-11T21:12:15Z
identity: security-20261111T211215-jay-flowers
tier: draft
---

When a deterministic gate mirrors an external component's configuration resolution, enumerate every path the component could read instead of a precedence-picked one, and refuse on any resolver input the gate cannot observe. dewey #191 (PR #196) fixed a fail-open where `dewey gate auto-answer` resolved the opencode-auto-question plugin config XDG-first while the plugin's src/config.ts always reads join(homedir(), ".config", "opencode") — a divergent XDG_CONFIG_HOME made the gate verify a file the plugin never reads while the plugin's real config went unchecked. The fix checks the home path always (authoritative for the reported list), adds the XDG path as a refusal-only input when lexically divergent, refuses on unset/empty/relative HOME even when XDG is set (the plugin's Node homedir() passwd fallback is unobservable to the gate — proceed only on inputs both processes can see), and dedupes candidate paths by lexical comparison rather than symlink evaluation: path aliasing can only over-check the same physical file, never under-check two different files as one.
