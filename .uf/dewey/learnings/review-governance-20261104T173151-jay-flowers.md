---
tag: review-governance
author: jay-flowers
category: decision
created_at: 2026-11-04T17:31:51Z
identity: review-governance-20261104T173151-jay-flowers
tier: draft
---

When a convention pack rule (e.g., meta-prompt MP-002/MP-014 requiring FR-NNN identifiers and a numbering allocation header in OpenSpec deltas) conflicts with a repository's uniformly established artifact format, review-time enforcement should treat the repo-wide convention as the baseline rather than flagging individual changes. In dewey, every OpenSpec delta (dewey-publish, mcp-internal-store-access, add-utf8-truncation-regression-test, extract-vertex-synthesis-constants, dewey-distill) uses prose `### Requirement:` headings with no FR ids; flagging only the newest change would penalize consistency. The pack-rule divergence is a governance decision for the whole corpus, best routed to a deliberate migration rather than per-PR findings.
