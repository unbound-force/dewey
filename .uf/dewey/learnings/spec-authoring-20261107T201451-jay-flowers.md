---
tag: spec-authoring
author: jay-flowers
category: pattern
created_at: 2026-11-07T20:14:51Z
identity: spec-authoring-20261107T201451-jay-flowers
tier: draft
---

When a change's behavior contract is a seam (dewey read-tools-bare-names: every GetPage/GetPageBlocksTree caller gains bare-name store resolution), spec and README prose should name the seam as the blast radius and mark consumer lists as illustrative ("including X, Y, Z; the blast radius is every <seam> caller"). The dewey #163 feedback round had to reword four documents (README, AGENTS.md, proposal.md, design.md) because prose had enumerated seam consumers exhaustively and journal_range was later recognized as part of the seam; the capability delta's non-exhaustive "including ..." requirement wording stayed conformant without edits. Exhaustive prose enumerations of seam consumers drift on the first new caller — name the seam, not the list.
