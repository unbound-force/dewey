---
tag: spec-authoring
author: jay-flowers
category: pattern
created_at: 2026-11-11T14:50:57Z
identity: spec-authoring-20261111T145057-jay-flowers
tier: draft
---

A requirement that defines a tool-call scope as 'A or B' without saying when each applies delegates an unstated routing judgment to the implementer. The gap becomes a coverage bug when a sibling requirement universalizes over the scope ('present verification for every candidate') and one option provably under-covers the set — e.g., a tags selector that cannot match pages whose namespace carries no tag prefix (knowledge/ pages match only via the tag property). Scope requirements should name a canonical selector that satisfies a stated coverage invariant, with alternatives permitted only where they demonstrably cover the same set.
