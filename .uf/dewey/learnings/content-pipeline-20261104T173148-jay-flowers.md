---
tag: content-pipeline
author: jay-flowers
category: gotcha
created_at: 2026-11-04T17:31:48Z
identity: content-pipeline-20261104T173148-jay-flowers
tier: draft
---

When a PR commits .uf/dewey/manifest.md, regenerate it after the FINAL code changes of the last fix round: the manifest's AST-derived MCP tool descriptions silently lag any registration-string changes made after the last `dewey manifest` run. In dewey PR 145 round 4, the G7 promote-ladder wording landed in server.go after the manifest's 2026-11-04T00:20:59Z generation, so the committed manifest described the pre-G7 contract while every other doc surface carried the final ladder. Review check: grep each manifest tool-description string against the live registration in server.go — a description that exists only in the manifest is stale.
