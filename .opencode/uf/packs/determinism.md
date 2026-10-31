---
pack_id: determinism
language: Any
version: 1.4.0
---

<!-- repo-local governance pack (not a scaffold asset);
     governance-styled per the content.md/ci.md precedent -->

# Convention Pack: Determinism (Constitution Principle VI)

This is the determinism governance pack for the Unbound Force
agent ecosystem. It codifies Constitution Principle VI —
Deterministic Execution — as review rules: agent context is a
scarce shared resource, and deterministic procedures MUST
execute in tools rather than be re-reasoned from prose.

Rules carry RFC 2119 markers with severity mapping to the
severity pack. Violations of the [MUST] rules (DE-001,
DE-003, DE-005, DE-007, DE-008) are CRITICAL findings; the
[SHOULD] rules (DE-002, DE-004, DE-006) are guidance that
is valuable but not universally applicable. The rules govern
new and modified instruction; existing prose is migrated
opportunistically at review time, not retroactively.

---

## Tool Extraction

- **DE-001** [MUST] Mechanical instruction added to commands,
  agents, or skills MUST be extracted to an executable tool
  with tests. A step is mechanical when it is a fixed
  procedure with the same output every time: parsing,
  existence checks, git choreography, output template
  rendering, enumeration loops. Judgment steps (decisions,
  evaluation, human interaction) remain in prose.

- **DE-002** [SHOULD] Extractions SHOULD preserve one or two
  tool output-format examples near the decision points that
  consume them, so consuming agents still recognize the
  rendered shape without re-carrying the instruction.

---

## Prescribed Output Rendering

- **DE-003** [MUST] Prescribed terminal output MUST be
  tool-rendered, not reconstructed from instructed templates.
  When a command's output format is prescribed, the tool that
  performs the procedure renders it; agents MUST NOT be
  instructed to reproduce the format from prose. Behavioral
  guards that verify rendered output are interim mitigations,
  not the mechanism of record.

---

## Efficiency Evidence

- **DE-004** [SHOULD] Change records SHOULD include
  measurable line/token deltas when instruction text changes,
  so efficiency claims are backed by before/after counts
  rather than impressions.

---

## Intent-Level Orchestration

Sequence-level collapse, known externally as the Router Pattern: the
agent outputs a single intent, and deterministic tool code maps it to
the required sequence of calls. Shipped precedent:
`uf_unleash_branch_gate`, `uf_unleash_detect_resume`,
`uf_delegate_command`, `dispatch_review_runs`.

- **DE-005** [MUST] A fixed sequence of tool calls with no judgment
  between steps — each next call and its arguments fully determined
  by the previous call's structured result — is itself mechanical
  instruction and MUST be collapsed into a single intent-level tool.
  The agent expresses the intent and passes judgment products
  (composed text, human gate decisions) as parameters; ordering,
  batching, retry, failure-stop, and branching on structured results
  belong to the tool. Instructing an agent to chain tools through a
  judgment-free sequence is a DE-001-class violation.

- **DE-006** [SHOULD] Intent-level tools SHOULD keep judgment at
  their boundary: judgment products arrive as validated parameters,
  and routing decisions (verdict mapping, severity sorting,
  exit-variant selection, batch scheduling, failure-stop) execute
  inside the tool as constants or pure functions. A tool whose
  contract requires the agent to pre-compute its internal routing
  re-introduces the orchestration DE-005 removes.

---

## Seam Verification

- **DE-007** [MUST] A prose→tool migration increment MUST
  verify the seams it creates in the same increment: the
  registration surface (tool registration plus input-schema
  parity), the documented output shape, and write→read
  round-trips. An increment whose seam tests are red or
  absent is not complete. At authoring time, the increment's
  tasks.md MUST include a seam-verification task naming the
  three layers (registration surface, documented output
  shape, round-trips). Shipped precedent: Layer 1
  registration-surface tests, Layer 2 golden output-shape
  tests, Layer 3 round-trip audits.

---

## Delegation Skill Enforcement

- **DE-008** [MUST] Work delegated through a delegation
  tool and classified as deterministic MUST carry a skill
  or script reference verified to exist, or a substantive
  recorded absence reason — one that states what was
  searched for or what was found (`n/a` and `not needed`
  are vacuous; a reason naming the searched locations and
  the absence found is substantive). Command, agent, and
  skill instruction that delegates work MUST bind
  mechanical work to repo tooling through the delegation
  tools' parameter contracts (`task_classification`,
  `skill`, `skill_absent_reason` on `uf_delegate_command`
  and on the forge subtask spawn tool
  `replicator_forge_spawn_subtask`), not through
  prose-only skill instructions. Swarm-path parallel
  delegations (`[P]` tasks spawned through the forge
  subtask tool) MUST pass the task classification and,
  for deterministic tasks, the verified skill or script
  reference or recorded absence reason as spawn-tool
  parameters; the spawned subtask description is not the
  contract's carrier. Both tools enforce the requirement
  fail-closed (`SKILL_REFERENCE_REQUIRED`,
  `SKILL_NOT_FOUND`) and record the classification,
  resolved reference, and reason; that record, not prompt
  archaeology, is the audit source. Delegating prose that
  names skills without the contract is an interim
  mitigation, not the mechanism of record.
