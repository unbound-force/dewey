---
tag: spec-review
author: jay-flowers
category: gotcha
created_at: 2026-11-02T20:37:31Z
identity: spec-review-20261102T203731-jay-flowers
tier: draft
---

Spec-review cross-reference checks must verify that each labeled pointer resolves inside the document the citation names, not merely that the id exists somewhere in the suite. In specs/017-openai-compatible-providers, plan.md's Structure Decision cited "research.md D1" while the alternatives-considered content lives in research.md R1; "D1" is plan.md's own Key Design Decisions label. The conflation survived two review rounds because the D-prefix looked plausible — plan.md and research.md use parallel but different numbering schemes (D-NNN vs R-NNN) for related content, and an id-validity check that ignores the cited document cannot catch the mix-up.
