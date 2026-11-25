---
tag: uf-workflow
author: jay-flowers
category: context
created_at: 2026-11-09T16:08:05Z
identity: uf-workflow-20261109T160805-jay-flowers
tier: draft
---

The uf git_push / git_push_force_with_lease plugin tools reject URL push targets by design (RemoteNameSchema forbids "/"), so commands that must push to fork-PR head remotes configured as URLs by gh pr checkout (uf.address-feedback §4.4) cannot use them and fall back to raw `git push <remote> HEAD:<merge-ref>` with prose-enforced charset gates and identity checks. The consolidation path is the #965 argument-array push tool: extend it to accept URL remotes so all uf commands share one tested push path and th
