---
tag: task-decomposition
author: jay-flowers
category: gotcha
created_at: 2026-11-11T15:17:33Z
identity: task-decomposition-20261111T151733-jay-flowers
tier: draft
---

A tasks.md step that both performs external work (e.g. filing a GitHub issue) and instructs "record its number inline here" edits tasks.md itself. When several such per-channel sync tasks are grouped, they must not all carry [P]: MP-017 forbids parallel markers on tasks that modify the same file, and concurrent execution can drop recorded issue numbers. Mark only the external calls parallel, or serialise the bookkeeping.
