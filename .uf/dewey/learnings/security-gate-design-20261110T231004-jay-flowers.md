---
tag: security-gate-design
author: jay-flowers
category: pattern
created_at: 2026-11-10T23:10:04Z
identity: security-gate-design-20261110T231004-jay-flowers
tier: draft
---

A fail-closed security gate must be fail-closed on the inability to verify, not only on the shape of located evidence. Name-based plugin detection that proceeds when "no entry exists" is fail-open for renamed plugins, alternate install directories, unscanned scopes, malformed or permission-denied config files, and unnormalized values; combined with an external marker contract that is not yet implemented, both defenses can be absent at once. Require positive attestation of what was scanned, define behavior for unreadable/malformed config, and add an acceptance case for a plugin enabled through an alternate path. Also reconcile assumed external config schemas with an observed sample before writing the gate procedure.
