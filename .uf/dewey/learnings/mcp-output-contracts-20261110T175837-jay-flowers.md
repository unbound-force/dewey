---
tag: mcp-output-contracts
author: jay-flowers
category: decision
created_at: 2026-11-10T17:58:37Z
identity: mcp-output-contracts-20261110T175837-jay-flowers
tier: draft
---

Remediation rule for shared-helper contract leaks (dewey PR #160, feedback item 2): when additive result metadata leaks from a shared helper (toSemanticResults) into a tool the spec did not name (`similar`), prefer documenting-and-testing — amend proposal/spec/README and pin with a test — over gating the helper at call sites. Rationale: validity/as_of are provenance metadata that Constitution III (Observable Quality) requires in query results; `similar` already carries tier/category/created_at through the same helper (013 FR-004 precedent), so gating creates an arbitrary per-tool metadata split and adds a code path with no security benefit; and the leak was enrichment-only with no ranking/filtering reach (Similar never calls finishSemanticResults), so there is no behavioral risk to gate. Reserve call-site gating for leaks that change ordering semantics or bypass a security control. Complements learning/intent-drift-20261108T233353-jay-flowers, which covers the drift gotcha itself.
