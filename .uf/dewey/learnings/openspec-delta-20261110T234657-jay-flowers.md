---
tag: openspec-delta
author: jay-flowers
category: gotcha
created_at: 2026-11-10T23:46:57Z
identity: openspec-delta-20261110T234657-jay-flowers
tier: draft
---

When a change adds a requirement that overrides the behavior of a requirement it MODIFIES in the same delta set, the MODIFIED text must be qualified to defer to the new requirement. serialize-supersession-writes iteration 2 restated learning-temporal-validity's Two-Way Supersession requirement with a "claim lands" qualifier but left its frontmatter clause unqualified ("the target MUST gain superseded_by ... in its frontmatter"), while the new SSW-FR-003 requires refusing the frontmatter write when the backup already names a different successor. The pre-fix fork-residue case (store unclaimed, backup already marked) triggers both MUSTs in opposite directions, so the delta set is internally contradictory even though design D3 had resolved the conflict toward refusal. Rule: after adding a guarded-write or refusal requirement, re-read every MODIFIED requirement whose behavior it constrains and add the explicit cross-reference in the normative text, not only in the design.
