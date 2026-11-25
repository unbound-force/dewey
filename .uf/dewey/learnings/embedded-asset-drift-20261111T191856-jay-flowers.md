---
tag: embedded-asset-drift
author: jay-flowers
category: pattern
created_at: 2026-11-11T19:18:56Z
identity: embedded-asset-drift-20261111T191856-jay-flowers
tier: draft
---

When an embedded asset is mirrored across copies (tracked file + embedded string literal + scaffolded output), copy-parity and byte-identity tests cannot detect a defect present in every copy — a shared typo or a ground-truth rename makes all copies agree while the entry is inert. Anchor one copy's identifiers against the source-of-truth registry instead: dewey's curator-tool-allowlist parses server.go's MCP tool registrations (registeredMCPToolNames) and fails the test when a frontmatter allow entry names a tool that is not registered, closing the drift direction parity tests are structurally blind to. Applies to any allowlist, manifest, or identifier set mirrored into embedded assets.
