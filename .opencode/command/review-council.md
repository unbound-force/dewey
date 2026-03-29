---
description: Run the reviewer governance council to audit codebase or spec compliance.
---
<!-- scaffolded by uf vdev -->
<!-- scaffolded by uf vv0.6.1 -->
# Command: /review-council

## User Input

```text
$ARGUMENTS
```

## Description

Review the current codebase **or** SpecKit artifacts for compliance with the Behavioral Constraints in `AGENTS.md` using the review council. The council dynamically discovers which reviewer agents are available rather than assuming a fixed set.

## Determine Review Mode

The review mode is determined automatically by examining the
workspace state. The user can also force a mode explicitly.

### Explicit Override

If `$ARGUMENTS` contains the word **"specs"**, use
**Spec Review Mode** regardless of auto-detection.

If `$ARGUMENTS` contains the word **"code"**, use
**Code Review Mode** regardless of auto-detection.

### Auto-Detection (when no explicit override)

When no mode keyword is provided, detect the mode by
examining the current branch and workspace:

1. **Get the current branch name**:
   ```bash
   git rev-parse --abbrev-ref HEAD
   ```

2. **Get the diff against the base branch** (`main`):
   ```bash
   git diff --name-only main...HEAD
   ```
   This shows all files changed on the current branch
   relative to `main`.

3. **Classify the changed files**:
   - **Spec files**: paths under `specs/`, `openspec/`,
     `.specify/`, or files named `spec.md`, `plan.md`,
     `tasks.md`, `checklists/`, `contracts/`,
     `data-model.md`, `research.md`
   - **Code files**: everything else (`.go`, `.ts`, `.js`,
     `.py`, `go.mod`, `go.sum`, `Makefile`, `internal/`,
     `cmd/`, `.opencode/agents/`, `.opencode/command/`,
     `.opencode/skill/`, `.opencode/unbound/packs/`,
     etc.)

4. **Detect the workflow tier** from the branch name:
   - Branch matches `opsx/*`: **OpenSpec** (tactical)
   - Branch matches `NNN-*` (digits then dash): **Speckit** (strategic)
   - Branch is `main` or other: no active workflow

5. **Select mode based on classification**:

   | Condition | Mode | Rationale |
   |-----------|------|-----------|
   | Code files changed | **Code Review** | Post-implementation -- review the code |
   | Only spec files changed | **Spec Review** | Pre-implementation -- review the specs |
   | No files changed vs main | **Spec Review** | On main or fresh branch -- review specs |
   | On `main` branch | **Spec Review** | No feature branch -- review specs |

6. **Announce the detected mode**: Always tell the user
   which mode was selected and why, including the
   workflow tier:
   > "Detected **Code Review Mode** (Speckit) — found N
   > code files changed on branch `012-swarm-delegation`
   > vs `main`."
   >
   > Or: "Detected **Spec Review Mode** (OpenSpec) — only
   > spec artifacts changed on branch
   > `opsx/documentation-accuracy`."
   >
   > Use `/review-council code` or `/review-council specs`
   > to override.

---

## Discover Available Reviewers

Before entering either review mode, discover which reviewer agents are available:

1. **Read the `.opencode/agents/` directory** using the Read tool to list all entries.

2. **Filter for Divisor persona agents**: from the directory listing, select only entries whose filename starts with `divisor-` and ends with `.md` (e.g., `divisor-adversary.md`, `divisor-architect.md`). Ignore subdirectories (entries ending with `/`) and non-matching files.

3. **Extract agent names**: for each matching file, strip the `.md` extension to get the agent name (e.g., `divisor-adversary.md` → `divisor-adversary`).

4. **Guard clause**: if zero Divisor persona agents are discovered, report to the user that no `divisor-*.md` agents were found in `.opencode/agents/` and stop. Do not proceed with either review mode.

5. **Note absent personas**: compare discovered agents against the known Divisor persona roles listed in the reference table below. Any known role not discovered is noted as absent. Absent personas are **informational only** — they do not block the review.

### Known Divisor Persona Roles (Reference Table)

This table documents known Divisor persona roles and their focus areas. It is used for context when delegating to discovered agents, but the **invocation list comes solely from discovery** — not from this table.

| Agent Name | Persona | Code Review Focus | Spec Review Focus |
|---|---|---|---|
| `divisor-adversary` | The Adversary | Security, resilience, efficiency, zero-waste, error handling, universal security, dependency vulnerabilities | Completeness, testability, ambiguity, security gaps, dependency risks, cross-spec consistency |
| `divisor-architect` | The Architect | Architectural alignment, coding conventions [PACK], pattern adherence, plan alignment, DRY, testing conventions [PACK], documentation [PACK] | Template consistency, spec-to-plan alignment, task coverage, data model coherence, inter-spec architecture |
| `divisor-guard` | The Guard | Intent drift detection, constitution alignment, neighborhood rule [PACK], zero-waste mandate | Intent fidelity, scope discipline, inter-spec consistency, status accuracy, user value, constitution alignment |
| `divisor-testing` | The Tester | Test architecture [PACK], coverage strategy, assertion depth, test isolation, regression protection, convention compliance [PACK] | Testability of requirements, test strategy coverage, fixture feasibility, coverage expectations, contract surface |
| `divisor-sre` | The Operator | Release pipeline [PACK], dependency health [PACK], configuration, runtime observability, upgrade paths, operational docs | Deployment feasibility, operational requirements, config management, dependency risk, maintenance burden |

For any discovered agent not in this table, delegate with a generic review prompt appropriate to the current review mode.

---

## Code Review Mode

Review the current codebase for compliance with the Behavioral Constraints in `AGENTS.md`.

### Instructions

1. **Replicate CI checks locally before delegating to council agents.** Read `.github/workflows/` to identify the exact commands CI runs, then execute those same commands. Any failure is a CRITICAL finding that must be fixed before the council review begins. Do not rely on a memorized list of commands — always derive them from the workflow files, which are the source of truth. This catches failures (e.g. linter violations) that code reading alone cannot reliably detect.

2. Delegate the review to all **discovered** reviewer agents in parallel using the Task tool. For each discovered agent, use the focus area from the Known Reviewer Roles reference table to provide targeted context. For any discovered agent not in the table, use a generic prompt: "Review the current changes for quality, correctness, and compliance. Return your verdict (APPROVE or REQUEST CHANGES) along with all findings."

   For each agent, instruct it to review the current changes and return its verdict (**APPROVE** or **REQUEST CHANGES**) along with all findings.

3. Collect all **REQUEST CHANGES** findings from the discovered reviewers. If all discovered reviewers return **APPROVE**, report the result and stop.

4. If there are **REQUEST CHANGES**, address the findings by making the necessary code fixes. Then re-run all discovered reviewers to verify the fixes. Repeat this loop until all discovered reviewers return **APPROVE** or the process has exceeded 3 iterations.

5. If 3 iterations are exceeded, ask the user whether to continue or stop.

6. Provide a final report to the user:
   - **Discovery summary**: how many reviewer agents were discovered, which were invoked, and which known reviewer roles were absent (informational, non-blocking)
   - What was found in each iteration
   - What was fixed
   - If stopped early, the current set of outstanding **REQUEST CHANGES**
   - If there were persistent circular **REQUEST CHANGES** (fixes for one reviewer cause failures in another), report those with additional detail so the user can make an informed decision

---

## Spec Review Mode

Review spec artifacts for quality, consistency, and
alignment with the project constitution. The review scope
depends on the detected workflow tier.

### Determine Review Scope

Based on the workflow tier detected in the auto-detection
step, determine which artifacts to review:

- **Speckit** (branch `NNN-*`): Review the active spec
  directory at `specs/NNN-<name>/` (spec.md, plan.md,
  tasks.md, contracts/, data-model.md, checklists/),
  plus `.specify/memory/constitution.md` and `AGENTS.md`.

- **OpenSpec** (branch `opsx/*`): Review the active
  change directory at `openspec/changes/<name>/`
  (proposal.md, design.md, specs/, tasks.md), plus any
  referenced main specs at `openspec/specs/`, plus
  `.specify/memory/constitution.md` and `AGENTS.md`.

- **No active workflow** (main or unknown branch): Review
  all spec artifacts across both `specs/` and
  `openspec/specs/`, plus the constitution.

### Instructions

1. Delegate the review to all **discovered** reviewer agents in parallel using the Task tool. For each discovered agent, use the focus area from the Known Reviewer Roles reference table (selecting the Spec Review Focus column) to provide targeted context. For any discovered agent not in the table, use a generic prompt: "Review the spec artifacts in scope for quality, consistency, and alignment. Return your verdict (APPROVE or REQUEST CHANGES) along with all findings."

   For each agent, instruct it to **operate in Spec Review Mode**: review the spec artifacts identified in the review scope above (not code), plus `.specify/memory/constitution.md` and `AGENTS.md`. Include the workflow tier (Speckit/OpenSpec) in the agent prompt so it can tailor its review accordingly. Instruct the agent to return its verdict (**APPROVE** or **REQUEST CHANGES**) along with all findings.

2. Collect all **REQUEST CHANGES** findings from the discovered reviewers. If all discovered reviewers return **APPROVE**, report the result and stop.

3. If there are **REQUEST CHANGES**, apply the **hybrid fix policy**:

   **Auto-fix (LOW and MEDIUM findings)** — Apply these fixes directly to the spec files:
   - Formatting and template compliance issues
   - Status field updates (e.g., "Draft" on a completed feature)
   - Terminology inconsistencies (same concept named differently across specs)
   - Missing or stale cross-references between spec, plan, and tasks
   - Coverage gaps with obvious fixes (e.g., a requirement with zero tasks when the task is clearly implied by the plan)
   - Stale or incorrect metadata (dates, branch names, prerequisite lists)

   **Report only (HIGH and CRITICAL findings)** — Do NOT attempt to fix these. Report them with full context and recommendations so the user can make an informed decision:
   - Missing user stories or acceptance criteria
   - Scope creep or under-specification
   - Design-level security gaps or unaddressed failure modes
   - Inter-feature conflicts or architectural misalignment
   - Constitution violations
   - Ambiguous requirements that require human judgment to resolve

4. After applying LOW/MEDIUM fixes, re-run all discovered reviewers to verify. Repeat this loop until all discovered reviewers return **APPROVE** (considering only remaining HIGH/CRITICAL findings as blocking) or the process has exceeded 3 iterations.

5. If 3 iterations are exceeded, ask the user whether to continue or stop.

6. Provide a final report to the user:
   - **Discovery summary**: how many reviewer agents were discovered, which were invoked, and which known reviewer roles were absent (informational, non-blocking)
   - What was found in each iteration
   - What was auto-fixed (LOW/MEDIUM)
   - Outstanding HIGH/CRITICAL findings that require human decision, with full context and recommendations
   - The Architect's Alignment Score for spec quality (if provided)
   - If there were persistent circular findings, report those with additional detail
   - Suggested next steps (e.g., "Run `/speckit.clarify` on spec 007 to resolve the ambiguous credential migration behavior")

---

## Verdict

The council returns **APPROVE** only when all discovered reviewers return **APPROVE**. Any single **REQUEST CHANGES** from a discovered reviewer means the council verdict is **REQUEST CHANGES**. Absent reviewers (known roles whose agent files were not found during discovery) do not affect the verdict but are noted in the discovery summary.

In Spec Review Mode, the council may return **APPROVE WITH ADVISORIES** when all LOW/MEDIUM findings have been auto-fixed but HIGH/CRITICAL findings remain that require human judgment. The advisories are the outstanding HIGH/CRITICAL findings. The discovery summary is included regardless of the verdict.
