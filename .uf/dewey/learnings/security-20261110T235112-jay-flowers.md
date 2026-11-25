---
tag: security
author: jay-flowers
category: pattern
created_at: 2026-11-10T23:51:12Z
identity: security-20261110T235112-jay-flowers
tier: draft
---

Fail-closed verdict matrices for config-reading security gates must cover the steps before the file read, not just the read itself: resolving the config directory (HOME/XDG both unset) and determining file existence (non-ENOENT stat errors such as a permission-denied parent directory). Specifying only "exists but unreadable/unparseable → refuse" lets an implementer map "cannot determine whether it exists" to the absent-file/pass branch, silently converting an unverifiable gate into a pass.
