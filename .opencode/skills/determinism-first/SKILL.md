---
name: determinism-first
description: Classify work as deterministic or judgment-based before writing. Route fixed procedures and judgment-free tool chains through executable tools instead of improvised prose output.
tags: [determinism, authoring, tools]
---

# Determinism First

Agent context is a scarce shared resource (Constitution
Principle VI, "Deterministic Execution"). Deterministic
procedures MUST execute in tools rather than be re-reasoned
from prose on every run. This skill is the authoring-time
check that keeps mechanical work out of instruction text.

## Classify Before Writing

Before writing or modifying a command, agent, skill, or any
instruction text, classify every step of the work:

- **Deterministic** — fixed procedure, same output every
  time. It belongs in a tool call (script, CLI command, or
  plugin), not improvised output.
- **Judgment-based** — a decision, an evaluation, or human
  interaction. It belongs in prose.

The test: if a competent agent follows the step twice, does
it produce the same result both times? If yes, the step is
deterministic and belongs in a tool. When classification is
uncertain, look at the concrete examples below before
defaulting to prose. The same test applies one level up: a fixed
sequence of tool calls with no judgment between steps is itself
deterministic.

## Deterministic Steps Belong in Tools

Mechanical procedures that MUST be tool calls, not
instructed prose:

- Parsing structured input (YAML, JSON, diffs, frontmatter)
- Existence and state checks (file present, branch current,
  tool available)
- Git choreography (worktrees, staging, merge-base diffs)
- Prescribed terminal output rendered from templates
- Enumeration loops over directories, files, or checks
- Orchestration of tool chains (ordering, batching, retry,
  failure-stop)
- Branching on structured results via lookup tables (verdict
  mapping, severity sorting)

When a deterministic step is needed:

1. Find an existing tool that performs it and call it.
2. If none exists, extract the procedure to an executable
   script or command with tests — do not restate the
   procedure in prose for the agent to re-derive.
3. Prescribed output MUST be tool-rendered, not
   reconstructed by the agent from a template embedded in
   instructions.
4. If the tool now sits in a judgment-free chain of calls,
   collapse the chain into a single intent-level tool; pass
   judgment products (composed text, gate decisions) as
   parameters.

When prose is extracted into a tool, preserve one or two
real output-format examples near the decision point that
consumes them, so later readers can still recognize correct
output. Behavioral guards written in prose are interim
mitigations, not the mechanism of record.

A prose→tool migration increment carries the discipline into
planning: its `tasks.md` MUST include a seam-verification
task naming the three layers — the registration surface
(tool registration and input-schema parity), the documented
output shape, and write→read round-trips. An increment whose
seam tests are red or absent is not complete. Rule DE-007
(`.opencode/uf/packs/determinism.md`) is the matching
review rule.

## Chains Without Judgment Collapse Too

Extraction does not stop at single steps. A fixed sequence of
tool calls with no judgment between steps — each next call and its
arguments fully determined by the previous call's structured result —
is itself a deterministic step and MUST be collapsed into one
intent-level tool whose code performs the chaining.

The twice-test applies one level up: if a competent agent executing
the chain between two judgment points produces the same calls in the
same order both times, the chain belongs in code. Re-reading and
re-sequencing ten tool calls on every run costs the same scarce
context as ten steps of prose choreography.

Judgment SHOULD stay at the boundary: composed content and human gate
decisions arrive as tool parameters; branching on structured results
(verdict mapping, severity sorting, exit-variant selection, batch
scheduling, failure-stop) happens inside the tool. Shipped
precedent: `uf_delegate_command` (command file read → prompt
assembly → child session spawn → artifact verification →
checkpoint) and `dispatch_review_runs` (plan → evidence → fan-out →
consolidate → artifact).

## Judgment Belongs in Prose

Decisions, evaluation, and human interaction are genuinely
judgment-based and stay in prose: choosing between designs,
triaging severity, weighing trade-offs, and interacting
with the user. Prose is the right medium only where the
output legitimately varies with context.

## Delegated Work Carries Its Classification

The twice-test does not stop at authoring. When work is delegated,
the classification travels with it: a delegation MUST declare
`task_classification` (`deterministic` or `judgment`), derived
from the nature of the work — never from routing metadata such as
workflow tier or route type. A `deterministic` delegation MUST
pass a `skill` reference verified to exist — a repo skill name or
a repo-relative script path — or record a `skill_absent_reason`
stating what was searched for and what was found; `n/a` and
`not needed` are vacuous.

The command delegation tool (`uf_delegate_command`) enforces the
contract fail-closed (`SKILL_REFERENCE_REQUIRED`,
`SKILL_NOT_FOUND`), injects the resolved skill reference into
the child prompt as a tool-rendered directive, and records the
classification, reference, and reason in checkpoint data and
the success envelope — that record, not prompt archaeology, is
the audit source. Judgment-classified delegations carry no skill
fields, and Swarm-path spawn calls for deterministic `[P]`
tasks pass the same fields as spawn-tool parameters
(`task_classification`, `skill`, `skill_absent_reason` on the
`replicator_forge_spawn_subtask` call). Rule DE-008
(`.opencode/uf/packs/determinism.md`) is the matching review
rule.

## Related Enforcement

- Constitution Principle VI, "Deterministic Execution"
  (`.specify/memory/constitution.md`)
- "Determinism gate" Behavioral Rule in `AGENTS.md`
- Review rules DE-001 through DE-007
  (`.opencode/uf/packs/determinism.md`)
- Review rule DE-008, "Delegation Skill Enforcement"
  (`.opencode/uf/packs/determinism.md`)
- The delegation tools' parameter contracts
  (`task_classification`, `skill`, `skill_absent_reason` on
  `uf_delegate_command` and on `replicator_forge_spawn_subtask`;
  `.opencode/plugins/uf-workflow/index.ts`,
  unbound-force/replicator)
