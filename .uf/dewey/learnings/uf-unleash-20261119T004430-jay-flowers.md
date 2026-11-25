---
tag: uf-unleash
author: jay-flowers
category: pattern
created_at: 2026-11-19T00:44:30Z
identity: uf-unleash-20261119T004430-jay-flowers
tier: draft
as_of: "dewey@f7c7f99f3d8f90dbf6d681b99a18c9f38162988c"
evidence:
  - "dewey#224"
  - "dewey:openspec/changes/record-which-dewey-built-an-index-and-sa/tasks.md"
---

When the orchestrator itself is a delegated child, the Task tool is blocked ("Subagent depth limit reached (1)"), so tasks.md [P] parallel batches cannot use Task-tool subagents. The working pattern: one forge worktree per task (replicator_forge_worktree_create at current HEAD, task_id like "task-4.1") plus exclusive comms file reservations, then concurrent uf_delegate_command children each pinned to its worktree path in the prompt (workers touch only their reserved file, never tasks.md), then replicator_forge_worktree_merge cherry-picks, scan_conflict_markers over the merged files, worktree cleanup_all, and the orchestrator marks checkboxes and commits the flips. Two-child and three-child batches both ran clean this way in dewey#224 phases 2 and 4.
