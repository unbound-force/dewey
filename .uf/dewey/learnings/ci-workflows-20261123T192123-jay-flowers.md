---
tag: ci-workflows
author: jay-flowers
category: pattern
created_at: 2026-11-23T19:21:23Z
identity: ci-workflows-20261123T192123-jay-flowers
tier: draft
as_of: "dewey@bc0159af2a897aa2b1283fbacd313a01a0808494"
---

Consolidating several release jobs into a single reusable-workflow call changes two operational properties that are easy to miss: (1) per-job `timeout-minutes` bounds are lost — the calling job inherits the 360-minute default, so a long/hangy step like `xcrun notarytool ... --wait` is no longer capped at the old per-job value unless the caller re-adds `timeout-minutes`; (2) the caller must grant the union of every internal job's permissions (`contents: write`, `checks: read`, `id-token: write`) because the called workflow's jobs cannot exceed the caller's grant — a benign widening but worth stating. Re-add an explicit `timeout-minutes` on the consolidated calling job to preserve failure containment.
