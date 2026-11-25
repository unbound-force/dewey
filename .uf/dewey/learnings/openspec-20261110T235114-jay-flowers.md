---
tag: openspec
author: jay-flowers
category: pattern
created_at: 2026-11-10T23:51:14Z
identity: openspec-20261110T235114-jay-flowers
tier: draft
---

When an OpenSpec change touches two capabilities that share one mechanism, assign the mechanism to a single capability's delta and cross-reference it by FR id from the other delta, with allocation headers recording disjoint id ranges (including ranges reserved for pending sibling changes that will archive later). This eliminated the contract-restatement DRY problem in dewey's curator-agent-human-gates iteration 2: the curator-agent delta owns the marker grammar (CA-FR-006) and gate tool (CA-FR-007), the knowledge-distillation delta references them by id from KD-FR-101/KD-FR-103, and KD-FR-001..099 stay reserved for the pending dewey-distill change whose requirements still carry names, not ids — so whichever change archives first cannot collide.
