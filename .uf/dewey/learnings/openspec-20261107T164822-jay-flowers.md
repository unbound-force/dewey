---
tag: openspec
author: jay-flowers
category: gotcha
created_at: 2026-11-07T16:48:22Z
identity: openspec-20261107T164822-jay-flowers
tier: draft
---

OpenSpec deltas in dewey diverge from the loaded meta-prompt pack: MP-002/MP-013 require capability-prefixed FR id lines opening each requirement body and MP-014 requires a requirement-numbering allocation header, but no delta under openspec/changes/ carries either — requirements are identified by "### Requirement:" headings alone. New changes must either adopt FR ids (inconsistent with all siblings) or the pack/practice divergence must be reconciled repo-wide; reviewers should flag it with that framing rather than as a change-local defect. Separately, openspec/config.yaml requires design.md to reference the proposal's constitution alignment and tasks.md to include a constitution verification task — most sibling changes carry both.
