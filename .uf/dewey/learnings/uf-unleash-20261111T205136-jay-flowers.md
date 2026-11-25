---
tag: uf-unleash
author: jay-flowers
category: pattern
created_at: 2026-11-11T20:51:36Z
identity: uf-unleash-20261111T205136-jay-flowers
tier: draft
---

Delegation playbook for nested /uf.unleash implement sessions (learned 2026-11-11 on branch opsx/dewey-distill-verify-step, dewey #151): (1) the Task tool fails with "Subagent depth limit reached (1)" inside an already-child session — [P] parallel tasks must fall back to sequential uf_delegate_command calls (worktree-per-task adds cleanup overhead with no benefit); (2) prompt-only uf_delegate_command without pack or model falls through the model-matrix classifier cascade, which can pick an unreachable endpoint (models.corp.redhat.com → "Cannot connect to API") — pass an explicit model (enmaas/rits/zai-org/glm-5-3) for non-Go work or pack: "go" for Go work; (3) the default 600s delegation timeout is too short for test-heavy tasks — a full-suite verification inside the child stalls and the run dies at the wall clock with nothing written; re-dispatch with timeout 1200000 and a leaner verification scope; (4) gaze-reporter and other Task-tool agents are equally depth-blocked — run the gaze CLI directly (gaze crap + gaze quality) and note the substitution in the checkpoint.
