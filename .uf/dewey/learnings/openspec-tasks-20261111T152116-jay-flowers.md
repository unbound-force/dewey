---
tag: openspec-tasks
author: jay-flowers
category: gotcha
created_at: 2026-11-11T15:21:16Z
identity: openspec-tasks-20261111T152116-jay-flowers
tier: draft
---

OpenSpec tasks.md header comments are cloned boilerplate: curator-agent-human-gates/tasks.md's comment promises "Phase checkpoints run the exact Build/Vet/Test steps (see each checkpoint)" and its body carries a **Checkpoint** block after every phase. gate-config-path-parity/tasks.md cloned the comment verbatim but dropped the blocks, leaving the comment pointing at structure the file lacks and Phase 1 ending with no test-suite run (MP-018 deviation). When cloning a sibling change's tasks.md scaffolding, diff the comment's promises against the body actually written — every structural claim in a copied header must be re-earned by the new file.
