---
tag: spec-governance
author: jay-flowers
category: gotcha
created_at: 2026-11-02T22:18:02Z
identity: spec-governance-20261102T221802-jay-flowers
tier: draft
---

When a review-iteration fix flips a status field in one spec artifact (e.g., spec.md Status: Draft/In Progress → Complete), the fix must sweep every artifact that mirrors the same status. In the dewey repo, AGENTS.md's Spec Organization list duplicates each spec's status in parentheses; updating spec.md alone ships an internally contradictory changeset. Pattern: after any status/metadata edit, grep the repo for the spec number (e.g., '017-openai-compatible') and align every mention that carries the stale value — spec.md, AGENTS.md spec list, and any checklist artifacts — in the same edit.
