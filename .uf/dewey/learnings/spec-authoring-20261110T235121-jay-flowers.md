---
tag: spec-authoring
author: jay-flowers
category: gotcha
created_at: 2026-11-10T23:51:21Z
identity: spec-authoring-20261110T235121-jay-flowers
tier: draft
---

Design documents that cite repository state as justification (a config key, a file's contents, a line anchor, a precedent pattern) must have that citation verified against the working tree at authoring time — grep the cited artifact before writing the citation. Symmetrically, review findings that claim a citation is false must also be verified against the working tree before acting on them: in curator-agent-human-gates iteration 2, a guard finding asserted design D2's `external_directory` ordering precedent "does not exist anywhere in the repo's opencode.json" and claimed a repo-wide grep found nothing, yet `.opencode/opencode.json:6` contains `agent.autonomy.permission.external_directory` with exactly the cited deny-first/allow-last ordering — the reviewer session lacked working-tree access. Verify falsification claims the same way you verify citations.
