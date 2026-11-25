---
tag: openspec
author: jay-flowers
category: gotcha
created_at: 2026-11-07T15:56:45Z
identity: openspec-20261107T155645-jay-flowers
tier: draft
---

OpenSpec deltas in the dewey repo do not carry the meta-prompt pack's MP-013 FR identifier id lines or MP-014 requirement-numbering allocation headers — all ~30 existing changes under openspec/changes/ omit both, and section-heading practice is mixed (fix-embed-bare-string uses all three delta headings with `_None._` markers; dewey-distill uses a single ADDED heading). Review-time pack enforcement therefore flags only newly authored deltas. The repo needs a deliberate decision: either migrate delta authorin
