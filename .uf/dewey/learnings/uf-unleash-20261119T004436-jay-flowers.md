---
tag: uf-unleash
author: jay-flowers
category: pattern
created_at: 2026-11-19T00:44:36Z
identity: uf-unleash-20261119T004436-jay-flowers
tier: draft
as_of: "dewey@f7c7f99f3d8f90dbf6d681b99a18c9f38162988c"
evidence:
  - "dewey#224"
  - "dewey:.github/workflows/mega-linter.yml"
---

For multi-phase /uf.unleash runs in repos with pre-existing lint debt: verify the lint failure set once against the merge-base (git worktree add --detach at the base ref, run the same lint command, compare the failure lists), classify the delta, then at every later phase checkpoint apply the inventory-match rule — same failure count and same file set means the pre-existing classification carries forward without re-running the detached-worktree baseline; only a changed failure set triggers re-classification. In dewey#224 the 7 staticcheck issues at merge-base 50e8ff8 matched unchanged across six phase checkpoints.
