---
tag: ci-path-gating
author: jay-flowers
category: gotcha
created_at: 2026-11-24T20:44:43Z
identity: ci-path-gating-20261124T204443-jay-flowers
tier: draft
as_of: "dewey@beae8a6b20b79a0d95b9a39662d536715396a1d5"
---

When a CI change gates a currently-required job behind a path-detection output, the merge-safety invariant is only preserved if the aggregate required check (ci-ok) and the always-on secret scan land in the SAME merge as the gate. A skipped required check satisfies branch protection, so merging the gate first (with the settings.yml swap deferred) opens a fail-open merge window. Verify the claim 'this PR carries the settings edit' against the actual changed-files list — not the design prose alone.
