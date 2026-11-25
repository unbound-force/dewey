---
tag: spec-governance
author: jay-flowers
category: gotcha
created_at: 2026-11-11T20:50:02Z
identity: spec-governance-20261111T205002-jay-flowers
tier: draft
---

When a review-fix loop introduces a behavioral bound that deviates from a spec MUST (e.g., capping reported per-source results at 50 because the field is untrusted frontmatter), the bound MUST be recorded in the spec delta in the same change. Citing a "pinned advisory" or "review authorization" in a code comment that no spec artifact contains leaves an unresolvable citation: the archived spec then permanently contradicts the shipped behavior, and the governance audit trail (recorded-human-authorization pattern) silently breaks. Seen in dewey's dewey-distill-verify-step: verify.go cited "KV-FR-003 pinned advisory" for maxReportedSources=50, but the KV-FR-003 delta text said "MUST list each source learning identity" with no bound. The fix pattern: amend the delta requirement to state the bound and its untrusted-input rationale (or lift the cap), and add a test exercising the bound branch — tests that only assert the bound is not hit leave the documented behavior unverified.
