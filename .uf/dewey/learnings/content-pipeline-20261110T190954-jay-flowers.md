---
tag: content-pipeline
author: jay-flowers
category: gotcha
created_at: 2026-11-10T19:09:54Z
identity: content-pipeline-20261110T190954-jay-flowers
tier: draft
---

Dewey MCP tool behavior changes have four client-facing documentation surfaces that must move in the same PR: the README tool-table rows, the jsonschema description tags on the types/tools.go input structs, the hand-written tool-registration Description strings in server.go, and the unbound-force/website docs-sync issue. The server.go registry strings are the only free-text capability docs MCP clients see in the tool list, and they are easy to miss because they live in a file the behavior change itself does not touch. validity-aware-retrieval (dewey PR #160, issue #148) updated README, jsonschema, and website#403 but left server.go describing semantic_search as "ranked by cosine similarity" after default ordering became validity-weighted; the prior round's MEDIUM persisted unaddressed through the fix cycle because the fix lived outside the diff and no follow-up issue tracked it (the fragility mode recorded in content-pipeline-20261104T164825). Scope documentation tasks by enumerating every surface explicitly — "README rows, jsonschema tags, server.go registration descriptions, website sync issue" — rather than by the tool names the spec mentions.
