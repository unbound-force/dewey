---
tag: openspec-spec-authoring
author: jay-flowers
category: pattern
created_at: 2026-11-11T00:05:25Z
identity: openspec-spec-authoring-20261111T000525-jay-flowers
tier: draft
---

When an OpenSpec change MODIFIES an existing capability's requirement and also introduces a new capability that restates part of the same contract, the two texts must be reconciled or they drift within one iteration. In dewey's serialize-supersession-writes, LTV-FR-001 was revised to include the losing-writer `supersedes_warning` MUST while SSW-FR-002 simultaneously claimed "this requirement adds only the warning" — leaving two capabilities owning the same MUST and an ownership sentence that is false. The deferral pattern already used in the same delta for the file leg ("subject to the guarded replace of ... SSW-FR-003") is the fix shape: state the outcome in exactly one requirement and have the other reference it by FR id. Reviewer check: for every requirement whose text says "adds only X", grep the other capability's requirement for X.
