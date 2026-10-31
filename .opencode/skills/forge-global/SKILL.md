---
name: forge-global
description: Cross-project forge coordination patterns
tags: [forge, global, coordination]
---

# Forge Global

Patterns for forge coordination that apply across projects.

## When to Forge

| Signal | Forge | Skip |
|--------|-------|------|
| File count | Task touches 3+ files | Task is a single-file change |
| Task structure | Independent subtasks that can parallelize | Sequential steps with tight coupling |
| Work type | Benefits from specialized workers (e.g., tests vs implementation) | Exploratory or investigative work |

## File Reservation Protocol

1. FIRST, workers MUST call `comms_reserve(paths=[...], ttl_seconds=300)` before editing any files (5-minute auto-release) — reservations are exclusive by default
2. THEN, always release when done: `comms_release(paths=[...])`
3. FINALLY, coordinator can emergency release if workers fail: `comms_release_all()`

## Worker Spawning

Each worker gets:
- A bead ID (cell in the org)
- An epic ID (parent cell)
- A list of assigned files
- Shared context from the coordinator

Workers operate independently and report back via comms.

## Broadcast

Coordinator can broadcast context updates to all workers:

```
forge_broadcast(
  project_path=".",
  agent_name="coordinator",
  epic_id="<id>",
  message="API contract changed, update imports"
)
```
