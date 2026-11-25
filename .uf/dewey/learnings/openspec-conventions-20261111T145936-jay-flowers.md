---
tag: openspec-conventions
author: jay-flowers
category: pattern
created_at: 2026-11-11T14:59:36Z
identity: openspec-conventions-20261111T145936-jay-flowers
tier: draft
---

Dewey openspec delta specs must carry a requirement-numbering allocation header and per-requirement capability-prefixed FR ids. The format was established by curator-agent-human-gates (CA-FR-001..008, KD-FR-101..103, with a header recording allocated ranges, reserved future ranges, and sibling-capability locations) and serialize-supersession-writes (SSW-FR-001..004 and LTV-FR-001, with SSW-FR-005..020 and LTV-FR-002..020 reserved), and is codified as MP-002/MP-013/MP-014 in .opencode/uf/packs/meta-prompt.md. Pre-#166 changes (dewey-distill, learning-temporal-validity) used named requirements only; curator-agent-human-gates' KD allocation note explicitly flags those as 'names, not ids'. A new delta that omits ids and the allocation header regresses the registry that prevents cross-change id collisions and makes requirements un-addressable by future deltas (verified while reviewing explicit-supersession-in-compile, whose 14 un-numbered requirements could not cite the SSW/LTV ranges they build on).
