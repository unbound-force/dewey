---
pack_id: meta-prompt
language: Any
version: 1.0.0
---
<!-- scaffolded by uf vdev -->

<!-- deployed scaffold asset (issue #889,
     model-matrix-scaffold-deployment change): uf init
     deploys a byte-identical copy of this pack to every
     hero repo through the embedded mirror and
     alwaysDeployedPacks entries (SC-FR-008), shipping a
     user-owned meta-prompt-custom.md initial-deployment
     template alongside it for project-specific meta-prompt
     rules in hero repos -->

# Convention Pack: Meta-Prompt (Spec, Plan, Tasks, Commands)

This is the meta-prompt convention pack for the Unbound Force
agent ecosystem. It contains the writing standards for agents
that author specification artifacts — spec.md, plan.md, and
tasks.md — and the command, agent, and skill files that
orchestrate work through them. Meta-prompting is authoring
the instructions that direct agents, rather than the code
those agents produce.

The pack is consolidation, not invention: every rule restates
a convention already documented in AGENTS.md, the
`.specify/templates/` templates, or the openspec change
format, so that spec-authoring work can carry it as a
loadable pack. It complements the language-agnostic
`default.md` pack and defers all determinism rules to
`determinism.md`.

In the delegation model matrix this pack maps to the
spec-authoring capability: plan and tasks delegations load it
before writing. Divisor review prompts load every pack in
.opencode/uf/packs/ through directory auto-discovery, so the
pack's rules also apply at review time.

Rules carry RFC 2119 markers. [MUST] rules are binding —
violations are review findings, and violations that break a
constitutional principle or a documented gate are CRITICAL
per the severity pack. [SHOULD] rules are strong guidance
that is valuable but not universally applicable; [MAY] rules
mark genuinely optional practice.

---

## Specification Language

- **MP-001** [MUST] Requirement statements MUST use RFC 2119
  keywords — MUST, MUST NOT, SHOULD, SHOULD NOT, MAY — in
  uppercase, and each keyword MUST carry its RFC 2119
  meaning. Advisory prose (context, rationale, examples)
  SHOULD NOT use the keywords casually.

- **MP-002** [MUST] Every functional requirement MUST carry a
  unique, stable FR-NNN identifier, sequential within its
  spec; success criteria number as SC-NNN where the speckit
  templates call for them. Existing identifiers MUST NOT be
  renumbered or reused when requirements are added. Speckit
  spec.md uses bare FR-NNN ids; OpenSpec deltas use
  capability-prefixed ids (for example, MM-FR-001)
  allocated per MP-014.

- **MP-003** [MUST] Acceptance criteria MUST be written as
  Given/When/Then scenarios with concrete, verifiable
  outcomes. Speckit user stories number their acceptance
  scenarios beneath the story; OpenSpec requirements each
  carry at least one `#### Scenario:` block with
  GIVEN/WHEN/THEN bullets. A scenario that cannot fail is
  not a scenario.

- **MP-004** [MUST] Prose lines in specification artifacts
  MUST wrap under 72 characters.

- **MP-005** [MUST] Specification and documentation prose
  MUST NOT contain literal, well-formed clarification-marker
  syntax. The shared marker grammar — a square bracket whose
  text begins with the two-word needs-clarification phrase,
  in its colon-terminated, bare-with-trailing-text, and
  completely bare forms — extracts as a question wherever it
  appears, including inside documentation, quoted text, and
  fenced examples. Prose that describes the grammar MUST
  describe it in words, as this rule does. Markers belong
  only in spec.md where a genuine clarify-phase question is
  intended; pluralized or unterminated marker-like text is
  ignored by design and MUST NOT be used as a substitute.

- **MP-006** [SHOULD] Specifications SHOULD capture boundary
  and error behavior — invalid input, absent configuration,
  concurrent use — as edge cases or explicit scenarios
  rather than leaving them to implementation judgment.

---

## Workflow Tiers and Artifact Shapes

- **MP-007** [MUST] Route every non-trivial change through a
  spec workflow tier before implementation: strategic work
  (three or more user stories, or cross-repo scope) uses
  Speckit — `specs/NNN-<name>/` artifacts on an `NNN-<name>`
  branch; tactical work (single repo, fewer than three
  stories) uses OpenSpec — `openspec/changes/<name>/`
  artifacts on an `opsx/<name>` branch; trivial fixes use
  the `quick/<name>` branch created by `/uf.unleash` tier
  routing. When in doubt, start with OpenSpec.

- **MP-008** [MUST] A spec MUST precede new features,
  refactoring that changes signatures, test additions across
  multiple functions, agent changes, CI changes, and
  data-model changes. Constitution amendments, typo fixes,
  and emergency hotfixes are exempt; hotfixes MUST be
  documented retroactively.

- **MP-009** [MUST] A change MUST escalate its tier when its
  scope grows beyond three user stories or crosses repository
  boundaries; it MUST NOT silently outgrow its artifact
  shape.

- **MP-010** [MUST] A Speckit feature directory MUST contain
  spec.md (prioritized user stories with acceptance
  scenarios, functional requirements), plan.md (the
  `/speckit.plan` output), and tasks.md (the
  `/speckit.tasks` output).

- **MP-011** [MAY] A Speckit feature directory MAY carry the
  plan-phase artifacts — research.md, data-model.md,
  quickstart.md, contracts/, and checklists/ — when the
  feature warrants them (a data model, a runnable
  quickstart, API contracts); placeholder artifacts MUST NOT
  be committed empty as ceremony.

- **MP-012** [MUST] An OpenSpec change directory MUST
  contain proposal.md (why, what changes, impact,
  constitution alignment), design.md (context, numbered
  decisions, risks), tasks.md, and one
  `specs/<capability>/spec.md` delta per capability touched.
  The change MAY record an originating issue number in
  `.openspec.yaml` to thread it through to the PR body.

- **MP-013** [MUST] An OpenSpec delta MUST organize its
  requirements under the three delta section headings —
  `## ADDED Requirements`, `## MODIFIED Requirements`, and
  `## REMOVED Requirements` — and nothing else. Each
  requirement is a `### Requirement:` heading whose body
  opens with an id line stating its FR identifier. A
  MODIFIED requirement MUST end with a Previously paragraph
  stating the contract it replaces; a REMOVED section with
  no removals MUST say so explicitly.

- **MP-014** [MUST] Each OpenSpec delta MUST open with a
  requirement-numbering allocation header recording the FR
  range this change allocates, the range reserved for future
  changes of the same capability, and where sibling
  capabilities' requirements live. Requirement ids MUST stay
  inside the allocated range.

- **MP-015** [MUST] The pipeline order is constitution →
  specify → clarify → plan → tasks → analyze → checklist →
  implement. Spec before plan, plan before tasks, tasks
  before implementation; spec artifacts MUST be committed
  and pushed before implementation begins. Each phase
  produces only its designated artifacts: specification
  phases write only within the feature directory, and
  implementation code is written only during implement.

---

## Plan and Task Decomposition

- **MP-016** [MUST] Every tasks.md task MUST be a `- [ ]`
  checkbox item with a stable identifier (Speckit T-NNN;
  OpenSpec phase-numbered ids such as 2.1), describing one
  actionable unit with exact file paths.

- **MP-017** [MUST] Mark a task `[P]` only when all three
  hold: it touches different files from the other `[P]`
  tasks in its group, it depends on no prior task in the
  group, and it can execute safely without ordering
  constraints. Tasks that modify the same file MUST NOT
  both carry `[P]`. Unmarked tasks run sequentially first,
  then `[P]` tasks run in parallel.

- **MP-018** [MUST] Group tasks under numbered phase
  headings in execution order, with foundational phases —
  shared infrastructure and blocking prerequisites — before
  feature phases. Each phase ends at a checkpoint that runs
  the test suite; the next phase MUST NOT begin while the
  checkpoint fails.

- **MP-019** [MUST] Checkbox discipline: flip a task to
  `[x]` immediately when it is complete — never batch
  completions, never pre-mark. A task is complete only when
  its own verification passes (tests, build, or lint as the
  task requires), not when its code is merely written.

- **MP-020** [SHOULD] Speckit tasks SHOULD be organized by
  user story (`[US1]`, `[US2]` tags) so each story is
  independently implementable, testable, and demonstrable.

- **MP-021** [SHOULD] A plan.md SHOULD carry a summary, the
  technical context, a Constitution Check gate that passes
  before research and is re-checked after design, the
  project structure with real paths, and a
  complexity-tracking table for any constitution violations
  that must be justified.

---

## Command and Instruction Files

- **MP-022** [MUST] Before writing or editing a command,
  agent, or skill file, classify each procedure it carries
  as deterministic or judgment. Deterministic work —
  parsing, existence checks, git choreography, template
  rendering, enumeration loops — MUST be routed to
  executable tools with tests, and delegations of it MUST
  carry the classification contracts. The determinism pack
  owns these rules: cite `determinism.md` (DE-001, DE-005,
  DE-008) by identifier rather than restating, duplicating,
  or weakening them here.

- **MP-023** [MUST] Command and task steps that gate on
  build, test, or lint MUST invoke the exact commands
  derived from the repository's CI configuration (for
  example, `make check` or `go test -race -count=1 ./...`),
  never paraphrased approximations; local verification MUST
  replicate the CI checks before work is marked complete.

- **MP-024** [SHOULD] When instruction is extracted to a
  tool, command prose SHOULD preserve the determinism
  pack's decision-point example discipline (DE-002): one
  or two rendered output examples near the decision points
  that consume them, so consuming agents still recognize
  the shape.

---

## Custom Rules

<!-- This section is intentionally empty in the canonical
     pack. A user-owned meta-prompt-custom.md
     initial-deployment template ships alongside it — the
     content.md and content-custom.md precedent for a
     governance-styled pack with a custom companion.
     Project-specific meta-prompt rules in hero repos
     belong in that sidecar; the unbound-force meta repo
     governs its own pack content through revision. -->
