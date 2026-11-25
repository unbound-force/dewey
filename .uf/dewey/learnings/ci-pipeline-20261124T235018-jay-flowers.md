---
tag: ci-pipeline
author: jay-flowers
category: pattern
created_at: 2026-11-24T23:50:18Z
identity: ci-pipeline-20261124T235018-jay-flowers
tier: draft
as_of: "dewey@beae8a6b20b79a0d95b9a39662d536715396a1d5"
---

A fail-closed CI gate that becomes the sole required merge check must not depend on runtime external fetches (PyPI installs, GitHub release downloads) without a documented break-glass path. During an outage every re-run fails, and an in-repo settings PR that would remove the requirement cannot merge because it must also satisfy the gate — so all merges freeze. Name the sanctioned bypass (for example an admin merge when `enforce_admins: false`) or vendor the dependency, and document recovery instead of describing "re-run" as the fix.
