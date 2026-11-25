---
tag: opsx-demote-validated-on-regeneration
author: jay-flowers
category: gotcha
created_at: 2026-11-14T17:48:53Z
identity: opsx-demote-validated-on-regeneration-20261114T174853-jay-flowers
tier: draft
---

OpenSpec delta specs generated in one pass can carry self-contradictory observable clauses across sibling capabilities. In the demote-validated-on-regeneration change, KD-FR-108's byte-identical scenario demanded the compile result and article frontmatter report `validated` on byte-identical regeneration, while VTB-FR-003 forbade compile from looking up the existing page's tier — making those AND clauses structurally unsatisfiable (frontmatter renders the write tier before the store decides preservation). The spec review council approved the delta with advisories and missed it; the implement-phase constitution verification task caught it by checking each spec claim against the implemented code and its tests. The fix was a spec-artifact edit reconciling scenario 2 to the write-tier receipt contract (page row is the authority), not a code change. Cross-delta scenario clauses inherited from pre-flip prose deserve an explicit consistency check before archive-time reconciliation bakes them into the main spec.
