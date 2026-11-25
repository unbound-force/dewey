---
tag: spec-conventions
author: jay-flowers
category: gotcha
created_at: 2026-11-07T19:02:36Z
identity: spec-conventions-20261107T190236-jay-flowers
tier: draft
---

In the dewey repo, OpenSpec delta requirements are majority bare-named: as of PR 157 (2026-11), 5 of 8 deltas under openspec/changes/ carry no FR-NNN identifiers, 0 of 8 have the MP-014 requirement-numbering allocation header, and the main capability specs under openspec/specs/ use bare "### Requirement:" headings too — even though the meta-prompt convention pack (MP-002/MP-013/MP-014, deployed 2026-11-01) makes FR numbering MUST-level for spec authoring. When reviewing dewey deltas, classify missing FR ids as a LOW spec-conventions advisory with disposition deferred to main-spec sync, not a blocking defect: partial compliance (id without allocation header) satisfies no MUST rule, and full MP-014 compliance needs a deliberate repo-wide FR-range allocation decision (or a meta-prompt-custom.md carve-out) before per-delta edits make sense. The one repo delta already using ids (index-pipeline-performance, FR-100/FR-101) shows the numbering style exists but was never adopted corpus-wide.
