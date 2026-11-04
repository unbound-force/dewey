---
tag: rebase-collisions
author: jay-flowers
category: gotcha
created_at: 2026-11-04T16:48:22Z
identity: rebase-collisions-20261104T164822-jay-flowers
tier: draft
---

When a long-lived branch that rewrites a shared file gets rebased over a feature that added helpers to the same file, keeping the rewrite wholesale silently deletes the merged helpers — and because the deleted helper's regression tests disappear with it, CI stays green while the behavior regresses. In this review, rebasing dewey-distill's rewrite of llm/config.go over 017's resolveGatewaySynthesisConfig dropped the documented DEWEY_SYNTHESIS_ENDPOINT fallback and its test, leaving only the config-wins direction pinned. Review heuristic: after a rebase onto a feature branch that touched the same files, diff the surviving test inventory against the merged feature's spec requirements — every MUST in the older spec should still have a green test that pins it, or an amended spec that retires it.
