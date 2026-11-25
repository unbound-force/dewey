---
tag: release-pipeline
author: jay-flowers
category: gotcha
created_at: 2026-11-23T19:21:20Z
identity: release-pipeline-20261123T192120-jay-flowers
tier: draft
as_of: "dewey@bc0159af2a897aa2b1283fbacd313a01a0808494"
---

When migrating a release pipeline onto a shared/central reusable workflow, the workflow_dispatch inputs the old pipeline supported are not automatically carried over. The dewey release.yml migration to jflowers/ship go-release.yml dropped `allow_prerelease: true` without mapping it to the new workflow or confirming the new default. Before merging a release-workflow migration, enumerate every input the old workflow forwarded (allow_prerelease, skip_*, ci_checks, tag) and verify each is either accepted by the new reusable workflow or intentionally removed and documented; otherwise documented capabilities (emergency-release bypass, prerelease support) silently regress.
