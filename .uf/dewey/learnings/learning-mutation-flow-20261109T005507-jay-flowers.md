---
tag: learning-mutation-flow
author: jay-flowers
category: pattern
created_at: 2026-11-09T00:55:07Z
identity: learning-mutation-flow-20261109T005507-jay-flowers
tier: draft
---

In dewey's tools package, learning-property mutations that must be mirrored to the markdown backup (validity changes, supersedes/superseded_by links) share one sequence: decode page properties (empty text and literal JSON null both normalize to an empty map), mutate, json.Marshal, store.UpdatePage, then setFrontmatterKeys on .uf/dewey/learnings/<identity>.md with a missing file or empty vaultPath tolerated. #146 shipped this as linkSuperseded/markSupersededFile (learning_temporal.go); PR #159's lint --fix path (markStaleUnverified/markStaleFile) re-implemented it step for step — copying even the marshal comment — and the two copies diverged on the failure contract (#146 returns a supersedes_warning string for a store-write-success/backup-write-failure; the lint path returns a bare error) before any shared helper existed. New mutation sites should extract and reuse a shared store+backup helper (e.g. updateLearningBackupFrontmatter(vaultPath, identity, keys)) with one partial-failure contract — store written, backup not — reported as a structured warning, rather than copying the flow again.
