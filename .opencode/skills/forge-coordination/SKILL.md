---
name: forge-coordination
description: Multi-agent coordination patterns for forge sessions
tags: [forge, coordination, multi-agent]
---

# Forge Coordination

Patterns for coordinating parallel agent work.

## Coordinator-Only Operations

- Coordinators MUST NOT reserve files
- Coordinators MUST NOT edit files directly
- Only coordinators MAY call `comms_release_all()` for emergency release of all reservations

### Coordinator Protocol

1. **Initialize**: `comms_init(project_path=".", task_description="...")`
2. **Check learnings**: `hivemind_find(query="<task keywords>")`
3. **Select strategy**: `forge_get_strategy_insights(task="<task>")`
4. **Decompose**: `forge_decompose(task="<task>", context="<learnings>")`
5. **Create epic**: `org_create_epic(epic_title="<task>", subtasks=[...])`
6. **Spawn workers**: `forge_spawn_subtask(bead_id, epic_id, subtask_title, files, task_classification, skill | skill_absent_reason)`
   - `task_classification` is required (`deterministic` | `judgment`)
   - `deterministic` subtasks must supply a verified `skill` reference or a `skill_absent_reason` (mutually exclusive); spawn fails closed with `SKILL_REFERENCE_REQUIRED` / `SKILL_NOT_FOUND`
   - `judgment` subtasks must omit both `skill` and `skill_absent_reason`; forward the envelope's `skill_directive` verbatim (or pass the same `skill` to `forge_subtask_prompt`) — never write skill instructions yourself
7. **Monitor**: `comms_inbox()` + `forge_status(epic_id, project_key)`
8. **Review**: `forge_review(task_id, files_touched)` for each worker
9. **Complete**: `forge_complete(bead_id, summary, files_touched)`
10. **Learn**: `hivemind_store(information="...", tags="forge,<topic>")`

## Worker-Only Operations

- Workers MUST reserve files before editing
- Workers MUST use `exclusive=true` when reserving files
- Workers MUST NOT call `comms_release_all()`
- Workers MUST release files when done: `comms_release(paths=[...])`

### Worker Protocol

1. **Initialize**: `comms_init(project_path=".", task_description="...")`
2. **Check learnings**: `hivemind_find(query="<task keywords>")`
3. **Reserve files**: `comms_reserve(paths=[...], exclusive=true, reason="...")`
   If reservation fails:
   a. Check who holds the reservation
   b. Send a message via `comms_send` to negotiate release
   c. Wait for release or escalate to coordinator
4. **Implement**: Make changes to reserved files only
5. **Report progress**: `forge_progress(bead_id, progress_percent, status)`
6. **Store learnings**: `hivemind_store(information="...", tags="...")`
7. **Release files**: `comms_release(paths=[...])`
8. **Complete**: `forge_complete(bead_id, summary, files_touched)`

## Progress Reporting

Report at milestones: 25%, 50%, 75%, 100%

```
forge_progress(
  project_key="replicator",
  agent_name="worker-1",
  bead_id="<id>",
  status="in_progress",
  progress_percent=50,
  message="Implemented core logic, starting tests"
)
```
