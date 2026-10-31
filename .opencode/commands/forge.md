---
description: Decompose task into subtasks and coordinate parallel agents
---

<protect>
# /forge

Decompose a task and spawn parallel workers.

## Critical Invariants

These rules are non-negotiable and MUST NOT be skipped:

- **Review MUST complete before marking work done** — step 7 (review) MUST finish before step 8 (complete). NEVER skip review.
- **The review gate is mandatory** — `forge_complete` MUST NOT be called until `forge_review` has passed for every worker. NEVER bypass the review gate.
- ALWAYS create a forge, even for small tasks.
- Coordinator orchestrates, workers execute — workers MUST NOT call `forge_complete`.

## Task

$ARGUMENTS

## Workflow

1. Initialize comms: `comms_init(project_path=".", task_description="Forge: <task>")`
2. Check prior learnings: `hivemind_find(query="<task keywords>")`
3. Decompose: `forge_decompose(task="<task>", context="<learnings>")`
   - Before decomposing, check historical success rates: `forge_get_strategy_insights(task="<task>")`
   - Choose from: `file-based`, `feature-based`, `risk-based`, or `auto`
4. Create epic: `org_create_epic(epic_title="<task>", subtasks=[...])`
5. For each subtask: `forge_spawn_subtask(bead_id, epic_id, subtask_title, files, task_classification, skill | skill_absent_reason)`
   - `task_classification` is required: `deterministic` or `judgment`
   - `deterministic` subtasks require a `skill` reference (verified skill name or repo-relative script path) OR a `skill_absent_reason` — never both
   - `judgment` subtasks must omit both `skill` and `skill_absent_reason` — supplying either fails input validation
   - Unverifiable references fail closed (`SKILL_NOT_FOUND`); missing both fails with `SKILL_REFERENCE_REQUIRED`
   - Pass `project_path` when spawning against a repo other than the current directory
   - Forward the envelope's `skill_directive` verbatim into the worker prompt (or pass the same `skill` to `forge_subtask_prompt`); never write skill instructions yourself
6. Monitor: check `comms_inbox()` every few minutes
   - `comms_inbox()` — check for messages from workers
   - `forge_status(epic_id, project_key)` — check worker progress
   - `org_cells(status="in_progress")` — see active cells
   - If a worker is blocked: read the message with `comms_read_message(message_id)`, acknowledge with `comms_ack(message_id)`, then either unblock or reassign the subtask
7. Review FIRST (before complete): `forge_review(task_id, files_touched)` for each completed worker — MUST finish before step 8
8. Complete (after ALL reviews pass):
   - `forge_complete(bead_id, summary, files_touched)` — mark epic done
   - `forge_record_outcome(bead_id, duration_ms, success)` — record for learning
   - `hivemind_store(information="...", tags="forge,<topic>")` — store learnings
   - `org_sync()` — persist state to git

## Rules

- Review every worker's output before marking complete — NEVER skip this step
- ALWAYS create a forge, even for small tasks
- Coordinator orchestrates, workers execute — workers MUST NOT call `forge_complete`
- Workers reserve their own files via `comms_reserve`
- Check inbox regularly for blocked workers
- Store learnings after completion
</protect>
