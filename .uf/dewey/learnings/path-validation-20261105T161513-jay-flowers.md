---
tag: path-validation
author: jay-flowers
category: pattern
created_at: 2026-11-05T16:15:13Z
identity: path-validation-20261105T161513-jay-flowers
tier: draft
---

In dewey, learning identities have exactly two origins with different trust levels: store_learning tool-created identities are strictly [a-z0-9-] (tag passes normalizeTag's [^a-z0-9-] strip, author passes the same strip inside resolveAuthor, timestamp is 20060102T150405 digits, collision suffix is -N), while reIngestLearnings inserts pages named learning/<identity> verbatim from untrusted git-synced frontmatter with only an emptiness check. Enforce the identity grammar at both boundaries rather than trusting "page exists with source learning": (1) reject invalid identities in resolveSupersededTarget inside prepareTemporal, so the invalid-input-writes-nothing invariant holds and the error returns before any write; (2) skip-and-warn malformed identities in reIngestLearnings so forged pages never enter the store, which also protects every other consumer of learning page names (e.g. promote's learning/ resolution). A ^[a-z0-9-]+$ allowlist rejects no legitimate identity in any historical format (old {tag}-{sequence} and current {tag}-{ts}-{author} are both inside the class), so it is a complete fix, not a heuristic. Pin with regression tests that plant identity: ../../evil in a learnings dir and assert re-ingestion skips it and store_learning refuses to supersede it with nothing written.
