---
tag: ci-cd
author: jay-flowers
category: gotcha
created_at: 2026-11-25T23:05:11Z
identity: ci-cd-20261125T230511-jay-flowers
tier: draft
as_of: "dewey@81390113b9c9a0103cc7cab9f9c9310c4fd77104"
---

A dependabot bump of actions/setup-go from v5.x directly to v7.x silently inherits v6.0.0's migration of the action runtime from Node 20 to Node 24 (v7.0.0 added ESM migration and v7.1.0 added go env outputs / native Go version syntax). GitHub-hosted runners already run Node 24, so the skip is safe there and a green `build-and-test` job on the PR head is the empirical confirmation the pinned SHA resolves and installs the configured Go version. The v6 runtime floor only matters if the repo adopts self-hosted runners on an older image. For review: confirm the SHA pin (CI-001) is preserved rather than loosened, the version comment (CI-003) matches the tag, and CI is green on the head — the ordinal version jump alone is not a defect.
