---
tag: upgrade-path
author: jay-flowers
category: gotcha
created_at: 2026-11-27T21:00:25Z
identity: upgrade-path-20261127T210025-jay-flowers
tier: draft
as_of: "dewey@f77093acbbe0468d0a190f8705f89f51eec3b341"
---

Removing a shipped scaffold or scheduled-automation artifact needs a user-facing upgrade note, not just a spec-artifact decision record. When a tool-owned scaffold mechanism has no retirement path (design explicitly defers removing previously scaffolded files), adopters keep the orphaned file, and any copied live automation keeps firing with no consumer. Pair the removal with an "Upgrading from the earlier template" section in the README naming the files to delete and the new scheduling model, and record it in release notes.
