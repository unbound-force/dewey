---
tag: agent-permissions
author: jay-flowers
category: pattern
created_at: 2026-11-11T15:32:38Z
identity: agent-permissions-20261111T153238-jay-flowers
tier: draft
---

When a spec's security boundary depends on unverified host behavior, the fallback path must be specified as a first-class spec-compatible branch: define the alternative shape, enumerate the requirements and tests the fallback invalidates, and add explicit tasks to amend them (requirement text, shape tests, embedded/scaffolded mirrors). A fallback that contradicts the requirement's mandated shape cannot be executed without unauthorized spec improvisation — the implementer is forced to either break the spec's letter or improvise amendments no task authorizes. Seen in the curator-tool-allowlist OpenSpec review: CA-FR-009 mandated a glob deny shape that the requirement's own fallback scenario replaced, with no reconciliation task.
