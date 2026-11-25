---
tag: opsx-demote-validated-on-regeneration
author: jay-flowers
category: context
created_at: 2026-11-14T17:49:00Z
identity: opsx-demote-validated-on-regeneration-20261114T174900-jay-flowers
tier: draft
---

Delegation infrastructure notes from running /uf.unleash on the dewey repo (2026-11-14): the opsx-propose entry command needs more than 30 minutes of wall clock (timed out twice — at 10 and 30 minutes — while still actively writing artifacts; resume by re-delegating with explicit "do not regenerate completed artifacts" context); per-task cobalt-crush-dev delegations fit comfortably in a 20-minute budget. Delegations without a pack parameter can resolve through the model-matrix classifier cascade to an unreachable endpoint (models.corp.redhat.com connection refused) — passing an explicit model (enmaas/rits/zai-org/glm-5-3) fixes it deterministically. The Task tool hits a subagent depth limit inside a parented session, so /uf.unleash's worktree parallel path cannot spawn workers there and falls back to sequential uf_delegate_command execution for [P] tasks.
