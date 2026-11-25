---
tag: spec-review
author: jay-flowers
category: gotcha
created_at: 2026-11-11T15:32:59Z
identity: spec-review-20261111T153259-jay-flowers
tier: draft
---

Design-rationale citations of repository artifacts (config paths, maps, test names, workflow commands) are verifiable claims, not decoration. Grep the cited path before shipping a design document: a precedent citation that points at a nonexistent file (e.g., citing an `.opencode/opencode.json` external_directory map when the repo's config lives at root `opencode.json` with no such map) undermines a design whose own standard is "verified, not assumed", and reviewers must treat every such citation as a checkable fact.
