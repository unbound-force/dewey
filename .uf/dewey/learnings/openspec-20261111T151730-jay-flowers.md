---
tag: openspec
author: jay-flowers
category: gotcha
created_at: 2026-11-11T15:17:30Z
identity: openspec-20261111T151730-jay-flowers
tier: draft
---

When an OpenSpec delta header cites a sibling capability requirement id, copy the path from the delta that actually allocates the id rather than from the sibling change's base directory. In dewey, the LTV capability's base spec (openspec/changes/learning-temporal-validity/specs/learning-temporal-validity.md) is un-numbered, while LTV-FR-001 is defined in the serialize-supersession-writes delta at openspec/changes/serialize-supersession-writes/specs/learning-temporal-validity/spec.md. A pointer assembled from the change name alone produced a nonexistent path and misidentified the defining document.
