---
tag: learning-identity
author: jay-flowers
category: gotcha
created_at: 2026-11-11T15:17:28Z
identity: learning-identity-20261111T151728-jay-flowers
tier: draft
---

Dewey learning identities have two valid on-disk formats: the current {tag}-{YYYYMMDDTHHMMSS}-{author} and the legacy {tag}-{seq} (auth-1, vault-walker-2). reIngestLearnings still recovers legacy files, and extractTagFromIdentity must parse both formats (learning-identity-collision-fix). Any new grammar that must recognize a learning identity — citation extraction, temporal-link rendering, source validation — must accept both formats or explicitly report the values it withholds. A timestamp-bearing grammar silently drops legacy identities, turning real supersedes links into "absent" and real citations into "uncited", which is invisible because both failure paths are fail-open.
