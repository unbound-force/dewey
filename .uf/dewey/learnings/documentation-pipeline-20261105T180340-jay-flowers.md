---
tag: documentation-pipeline
author: jay-flowers
category: gotcha
created_at: 2026-11-05T18:03:40Z
identity: documentation-pipeline-20261105T180340-jay-flowers
tier: draft
---

AGENTS.md carries per-tool API contract sections that go stale when tool inputs change, and docs-impact assessments conditioned on "structure changes" miss them. dewey PR #156 added evidence/validity/supersedes inputs and the automatic as_of field to store_learning, updated README and the MCP tool description, and left AGENTS.md's Store Learning API section (which enumerates that tool's inputs and automatic fields) documenting the old contract.
