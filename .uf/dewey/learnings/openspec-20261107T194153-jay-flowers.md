---
tag: openspec
author: jay-flowers
category: gotcha
created_at: 2026-11-07T19:41:53Z
identity: openspec-20261107T194153-jay-flowers
tier: draft
---

In the dewey repo, OpenSpec delta specs follow the repo-local schema template at openspec/schemas/unbound-force/templates/spec.md (## ADDED/MODIFIED/REMOVED Requirements sections, '### Requirement:' headings, RFC 2119 text, Previously paragraph) — with NO FR-identifier lines and NO requirement-numbering allocation header. The meta-prompt convention pack's MP-002/MP-014 FR-id and allocation-header rules are not this repo's established openspec practice: a grep of openspec/changes/ found no allocation headers, and both flat (specs/<name>.md, ~26 changes) and directory (specs/<capability>/spec.md, 4 changes) delta layouts have precedent. Architect reviews of openspec/changes/* deltas in dewey should calibrate against the repo template and sibling changes rather than the meta-prompt pack's numbering rules, or they will raise false-positive convention findings.
