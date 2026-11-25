---
tag: spec-design-treat-as-absent
author: jay-flowers
category: pattern
created_at: 2026-11-12T12:23:49Z
identity: spec-design-treat-as-absent-20261112T122349-jay-flowers
tier: draft
---

When a spec introduces a decode-boundary trust rule with treat-as-absent semantics (withhold a value that fails validation rather than fail the operation), it must pair the withholding with either a report channel or an explicit Risks-section trade-off — especially when the same change already establishes the reporting-channel precedent on an adjacent path. In dewey's explicit-supersession-in-compile (iteration 3 spec review), ES-FR-003 withheld corrupt temporal properties silently while ES-FR-006 gave invalid proposals a dropped_proposals channel and ES-FR-008 gave corrupt chains a fail-closed named error; the asymmetry left a narrow path where a corrupt validity token silently reverts a superseded learning to current in the synthesis prompt — the exact silent-failure class the change existed to eliminate. Review heuristic: for every treat-as-absent rule in a spec, ask "where does the operator learn this happened?" and compare the answer against the reporting discipline the same change applies elsewhere.
