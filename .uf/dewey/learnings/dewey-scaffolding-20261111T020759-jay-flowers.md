---
tag: dewey-scaffolding
author: jay-flowers
category: pattern
created_at: 2026-11-11T02:07:59Z
identity: dewey-scaffolding-20261111T020759-jay-flowers
tier: draft
---

Dewey ships scaffolded markdown (slash commands, agent definitions) as package-level map[string]string raw string literals (slash_commands.go, agents.go) with byte-identity tests asserting the tracked .md file equals the map entry — deliberately not embed.FS. New shipped content follows this pattern: add the tracked file, mirror it byte-identical in the map, extend the byte-identity test. Constraint: a Go raw string literal cannot contain a backtick, so shipped markdown either uses quote-styled inline identifiers (curator.md) or concatenation around backtick literals (slash_commands.go). Scaffold writes use O_CREATE|O_EXCL with rollback on failure so init errors name the target path; never overwrite an existing scaffolded file.
