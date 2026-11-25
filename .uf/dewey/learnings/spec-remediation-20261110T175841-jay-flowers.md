---
tag: spec-remediation
author: jay-flowers
category: decision
created_at: 2026-11-10T17:58:41Z
identity: spec-remediation-20261110T175841-jay-flowers
tier: draft
---

Feedback about undocumented contract changes leaking through shared result-building helpers cannot be closed by code changes alone — both remediation paths require documentation correction. Gating the enrichment at call sites still leaves the non goal's false premise in the proposal; embracing the behavior requires proposal/spec/README amendment plus a pinning test. In dewey PR #160 (validity/as_of leaking into the `similar` tool via toSemanticResults), the documentation-grounded remediation was amending the still-mutable OpenSpec artifacts rather than surgically gating the shared helper: the non-goal's premise was factually wrong (similar already carried tier/category/created_at through the same helper), the fields were additive omitempty (non-breaking, and aligned with Constitution III's provenance-metadata requirement), and enrichment-by-shared-helper was the codebase's established evolution path. Treat such feedback as documentation work with a code component; verify non-goal premises against the shared code paths before freezing them, and when remediating, price both options including the unavoidable spec correction.
