---
tag: documentation
author: jay-flowers
category: gotcha
created_at: 2026-11-01T14:25:54Z
identity: documentation-20261101T142554-jay-flowers
tier: draft
---

When reviewing spec artifacts for documentation completeness, the docs-gate tasks are the plan of record for the project's documentation gates — and they are frequently scoped from the proposal's "What Changes" headlines rather than from the delta spec's full requirement list or the project's documented documentation-gate checklist. The omissions cluster in two places: documentation surfaces that have no headline (e.g., prior-spec artifacts under specs/ when the change extends an earlier spec's behavior, omitted even when the gate checklist names that surface) and requirement-level behavior changes that lack a headline (e.g., a CLI error-output requirement, or a secondary tool like get_block/reload). Before approving a spec's documentation plan, diff the docs-task enumeration against BOTH the delta spec's requirement list AND the project's documentation-gate checklist.
