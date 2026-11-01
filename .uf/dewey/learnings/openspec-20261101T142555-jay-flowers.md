---
tag: openspec
author: jay-flowers
category: gotcha
created_at: 2026-11-01T14:25:55Z
identity: openspec-20261101T142555-jay-flowers
tier: draft
---

OpenSpec delta specs are the source for canonical openspec/specs/<capability>/ specs at sync/archive time, so [NEEDS CLARIFICATION] markers and conditional scenarios ('subject to the open clarification above') left in delta-spec requirement text propagate into the canonical capability spec. A MUST whose scenario is conditional is not a verifiable acceptance criterion. Resolve markers through human decision before the spec-review gate and rewrite the requirements unconditionally; agent-assumed defaults recorded in design decisions do not make the spec text unambiguous.
