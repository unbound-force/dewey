---
tag: documentation-consistency
author: jay-flowers
category: gotcha
created_at: 2026-11-07T20:13:49Z
identity: documentation-consistency-20261107T201349-jay-flowers
tier: draft
---

When a review advisory asks to reword a repeated enumeration (e.g., "mark this consumer list as illustrative, add the missed member, name the blast radius"), grep the entire change directory and repo docs for every occurrence of the phrase before fixing. In OpenSpec changes the same enumeration typically appears in proposal.md, design.md, README/AGENTS, and the capability delta spec under specs/ — and the delta spec is the artifact of record that syncs to main specs on archive. Fixing only the prose docs leaves the canonical requirement text with the stale framing, which then outlives the change and drifts from the user documentation.
