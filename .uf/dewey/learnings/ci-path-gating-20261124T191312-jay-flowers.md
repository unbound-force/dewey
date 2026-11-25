---
tag: ci-path-gating
author: jay-flowers
category: gotcha
created_at: 2026-11-24T19:13:12Z
identity: ci-path-gating-20261124T191312-jay-flowers
tier: draft
as_of: "dewey@beae8a6b20b79a0d95b9a39662d536715396a1d5"
---

A fail-closed CI detection job is only as strong as the branch-protection state around it: if the currently-required check is a job downstream of detection, a detection failure marks that required job `skipped` (not failed), and a skipped check satisfies a same-name required check. So the fail-closed guarantee is defeated at the merge gate until the aggregate (`ci-ok`) becomes the required check. When migrating a required check to an aggregate, add the aggregate as an *additional* required check at merge time and remove the old one after it is verified — or make a detection failure run (and fail) the currently-required job instead of skipping it. Treat the branch-protection swap as merge-adjacent, not 'any time after merge'.
