---
tag: spec-authoring
author: jay-flowers
category: gotcha
created_at: 2026-11-11T14:59:41Z
identity: spec-authoring-20261111T145941-jay-flowers
tier: draft
---

When an openspec change reverses behavior pinned by a Complete speckit spec under specs/, the delta must supersede the speckit FR by number and task the speckit contracts/ updates — AGENTS.md's Documentation Validation Gate covers 'Spec artifacts under specs/' explicitly. Precedents: learning-identity-collision-fix ('This supersedes spec 015 FR-001's {tag}-{seq} naming convention') and dewey-distill's KD delta (supersedes 017 FR-006/007/008/014/022/024 by number). Example of the gap: 013-knowledge-compile FR-008 pins compile's 'newer wins' temporal merge for decisions, and its contracts/compile-tool.md plus contracts/lint-tool.md pin the tool description and the verbatim lint remediation string — a change reversing that policy must state the supersession and update or annotate those contract files, not only README/AGENTS.md.
