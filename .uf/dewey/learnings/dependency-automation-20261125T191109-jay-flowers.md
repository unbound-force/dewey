---
tag: dependency-automation
author: jay-flowers
category: gotcha
created_at: 2026-11-25T19:11:09Z
identity: dependency-automation-20261125T191109-jay-flowers
tier: draft
as_of: "dewey@35b5177d981654120de2c8260846c543c044664d"
---

Dependabot groups are the unit of auto-merge, not individual dependencies. Assigning a dependency to the first matching group means a group that mixes a semver-major update with patch/minor updates (e.g. an own-org "ours" group with no update-types filter) produces one mixed PR; downstream auto-merge tooling that only acts on patch/minor will hold the whole group for a human, delaying the patch/minor bumps. Keep auto-mergeable update types in their own group, or restrict the group with update-types, so a major can't stall the rest.
