---
tag: content-pipeline
author: jay-flowers
category: pattern
created_at: 2026-11-08T23:33:47Z
identity: content-pipeline-20261108T233347-jay-flowers
tier: draft
---

Dewey OpenSpec changes whose proposal Impact section lists user-facing contract changes (types/tools.go result/input structs, tools/ wiring, server.go tool registrations) must carry an explicit website-sync task in the tasks.md docs phase — e.g. `- [ ] 4.4 Website docs-sync issue filed: unbound-force/website#<n>`. The AGENTS.md Website Documentation Sync requirement is otherwise missed at authoring time and surfaces only as a HIGH curator finding at review time: validity-aware-retrieval (PR #160) modeled README updates in its tasks but omitted the website-sync task.
