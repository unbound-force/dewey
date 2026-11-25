---
tag: spec-review-docs-tasks
author: jay-flowers
category: gotcha
created_at: 2026-11-12T12:23:46Z
identity: spec-review-docs-tasks-20261112T122346-jay-flowers
tier: draft
---

Spec-review verification of documentation-surface tasks: when a tasks.md docs task enumerates specific stale-text locations (file + ~line) or sets an absolute completion bar like "so no 013 document still states the retired rule as current", verify by grepping the retired phrasing across each target document instead of trusting the enumeration. Speckit specs restate every rule across independent layers — user-story prose, independent test, acceptance scenarios, functional requirements, success criteria — so an inventory anchored on the FR layer (FR-008 plus "its acceptance criterion 4") misses the other layers. In explicit-supersession-in-compile iteration 3 (dewey #150), task 9.6 named 013 spec.md FR-008 (~line 144) and AC-4 (~line 44) but grep found five more newer-wins restatements in the same file (lines 31, 37, 42, SC-002 ~190, SC-007 ~195), and task 9.1 missed README's temporal-validity row (~813, "Set by: supersedes input") even though the equivalent AGENTS.md bullet (~461) was tasked with the "reads as the only way to link learnings" rationale. The same grep also cheaply confirms every positively-cited surface (README 431/757, AGENTS.md 461, server.go 487, cli.go, lint.go, compile_test.go 706, research.md R7, T022, contracts) — grep the falsified phrasing, not the requirement id, and grep the sibling user-facing doc for any "only way" framing the change tasks in one of them.
