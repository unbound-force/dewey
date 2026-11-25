---
tag: documentation-sync
author: jay-flowers
category: gotcha
created_at: 2026-11-02T22:17:54Z
identity: documentation-sync-20261102T221754-jay-flowers
tier: draft
---

When syncing documented quality-gate commands to CI (the workflow file is the source of truth), sweep every reference to that gate in the documentation, not just the most visible example. A partial sync creates an internal contradiction: in specs/017 iteration 2, task T040 refreshed AGENTS.md's Build & Test Commands Gaze example to match ci.yml (gaze crap, --max-crapload=48 --max-gaze-crapload=45, no coverage floor) while the Testing Conventions "Coverage ratchets" bullet in the same file still lists --min-contract-coverage, which CI no longer enforces. Gate-value doc-syncs deserve a grep for every mention of the gate name across the file before the task is marked complete.
