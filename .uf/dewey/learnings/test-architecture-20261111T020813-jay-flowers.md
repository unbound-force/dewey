---
tag: test-architecture
author: jay-flowers
category: gotcha
created_at: 2026-11-11T02:08:13Z
identity: test-architecture-20261111T020813-jay-flowers
tier: draft
---

Byte-identity tests between tracked markdown policy files and their shipped string-literal mirrors (dewey's agents.go/slash_commands.go pattern) verify consistency, not content: a coordinated edit to both sides passes CI while silently changing the policy. For files whose content IS the security contract (tool flags, permission denials, mandatory-step instructions), pair the byte-identity test with structural assertions on the content itself — parse the frontmatter and assert the exact flag shape, assert the mandatory section names its command. Content assertions should target the invariants that fail open (over-permission, skipped gates), since fail-closed drift surfaces at runtime anyway.
