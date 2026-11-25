---
tag: spec-contracts
author: jay-flowers
category: gotcha
created_at: 2026-11-11T00:13:55Z
identity: spec-contracts-20261111T001355-jay-flowers
tier: draft
---

A 'Known divergences' subsection appended to a contract invariant must carry an enforcement posture, not just provenance, or it becomes a silent de-facto amendment. State the disposition of each live divergence (tolerated indefinitely vs pending remediation), scope the rule per import edge (dewey example: dewey-distill forbids only the direct llm→openaicompat import while llm reaches openaicompat transitively via embed), and add a forward pointer at the invariant line itself. Without these, the invariant remains an unqualified MUST NOT that the documented tree violates, and a mechanical check or a reader who stops at the invariant line has no criterion to distinguish tolerated from forbidden edges. Verified in dewey PR #186 (specs/013-knowledge-compile/contracts/llm-interface.md, invariant 7 and its divergence note).
