# Requirements Checklist: Weekly Unattended Distill with Rolling Per-Theme Review PRs

**Feature**: [018-weekly-distill-automation](../spec.md)
**Created**: 2026-11-15
**Purpose**: Validate spec requirements against constitution, completeness, and testability standards

## Content Quality

- [ ] No implementation details leak into spec (language, framework, file paths outside the specified artifact paths are absent; spec describes behavior, not code)
- [ ] Linked artifacts exist (issue #193, epic #153, dependencies #203/#204/#194 are real and accessible)
- [ ] Dependencies are explicitly stated (on #203 for git-is-source-of-record, #204 for stale tier, #194 for CODEOWNERS enforcement, jflowers/gh-hud#28 for dispatch, jflowers/opencode-supervisor#51 for agent profile)
- [ ] Out-of-scope section prevents scope creep (interactive distill, tier derivation, promotion manifests, gh-hud/supervisor internals, auto-merge, LLM judge tuning)
- [ ] All user stories are independently testable (each has an Independent Test paragraph describing a fixture-based verification)
- [ ] Success criteria are measurable and technology-agnostic (SC-001 through SC-008 each define a concrete pass/fail assertion)
- [ ] No more than 3 [NEEDS CLARIFICATION] markers (spec contains 0)
- [ ] Markdown renders correctly (headings, bold, lists, code spans all well-formed)

## Requirement Completeness

- [ ] FR-001 through FR-022 cover all 8 design points from issue #193 (trigger, scope, assignment/supersession, rolling PRs, merge-as-vouch, distiller agent, backlog run, interactive preservation)
- [ ] FR-005/FR-006 specify deterministic scope and question-tool prohibition (the core safety invariant for unattended mode)
- [ ] FR-011/FR-015 specify rolling PR dedup and human-commit preservation (the user-visible payoff and rebase safety rule)
- [ ] FR-016/FR-017 specify privilege narrowing from global config only (prevents repo-local escalation)
- [ ] FR-019 correctly scopes the validated-trust claim to #203's existing behavior (no new gate introduced)
- [ ] Edge cases cover empty scope, backlog volume, non-git repo, malformed policy, concurrent runs, manual edits, mid-review ingestion, and embedding outages
- [ ] Key Entities are defined (Distill Run, Scope Set, Distill Policy, Theme, Rolling PR, Review Queue, Distiller Agent) with relationships
- [ ] Assumptions trace to real prior work (#194 CODEOWNERS, #150 explicit supersession, #147 lint, #203/204 tier semantics, daily chore PRs cadence)
- [ ] Epic #153 governance rules are enumerated and each is traced to a scoped FR or assumption (demotion-not-promotion, guard-always-on, no guard weakening, auto-answer isolation, draft-clamp, git vouch, append-only supersession)

## Feature Readiness

- [ ] Spec is unambiguous enough for an engineer to plan without asking the author (no [NEEDS CLARIFICATION] markers, edge cases documented, acceptance scenarios are Given/When/Then)
- [ ] Acceptance scenarios in each user story map to FRs (US1→FR-001..FR-004, US2→FR-005..FR-007, US3→FR-008..FR-010, US4→FR-011..FR-015, US5→FR-016..FR-017, US6→FR-018..FR-019, with FR-020..FR-022 cross-cutting)
- [ ] Potential conflicts with in-flight specs identified (e.g., any other spec modifying `.uf/dewey/compiled/**` tier handling or `dewey-distill` slash commands)
- [ ] Constitution alignment checked: I (Composability First) — workflow template is additive, no hard dependency on gh-hud/supervisor; II (Autonomous Collaboration) — all interaction via git artifacts and MCP tool calls; III (Observable Quality) — rolling PRs and review queue bodies are the audit surface, logs include provenance; IV (Testability) — every story has a fixture-based Independent Test
