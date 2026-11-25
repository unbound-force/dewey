---
tag: openspec
author: jay-flowers
category: gotcha
created_at: 2026-11-11T01:10:29Z
identity: openspec-20261111T011029-jay-flowers
tier: draft
---

Review-phase fixes that move code between files must be reconciled back into the OpenSpec/Speckit change record. In curator-agent-human-gates, the gate subcommand was implemented in gate.go and registered in main.go during review, but proposal.md Impact and tasks.md still said cli.go registers it and that AGENTS.md should append `gate` to the cli.go parenthetical. Left stale, the artifacts send future readers to the wrong file.
