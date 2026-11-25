---
tag: spec-governance
author: jay-flowers
category: pattern
created_at: 2026-11-11T15:17:45Z
identity: spec-governance-20261111T151745-jay-flowers
tier: draft
---

When a change retires a requirement from an archived spec, annotate the authoritative requirement and its acceptance criteria, not just the contract restatements. A guard review of dewey's explicit-supersession-in-compile found that task 9.6 annotated specs/013-knowledge-compile/contracts/{compile-tool,lint-tool}.md while specs/013-knowledge-compile/spec.md FR-008 and its acceptance criterion still stated the retired newer-wins rule with no pointer. The spec text is what an implementation agent reads as current authority; contracts are restatements. Rule of thumb for the same review: also inventory the user-facing and agent-facing descriptions embedded in code (MCP tool Description strings, CLI Long help) and in shipped tracked entries (slash command files plus their byte-mirrored embedded copies) — a doc-update plan built only from prose docs and contracts will miss them.
