---
tag: review-dispatch
author: jay-flowers
category: gotcha
created_at: 2026-11-16T00:37:37Z
identity: review-dispatch-20261116T003737-jay-flowers
tier: draft
---

When dispatching a Divisor review council via dispatch_review_runs, the session_metadata.plan array must transcribe each plan entry's model string exactly as plan_review_dispatch returned it. On the 018-weekly-distill-automation code review (2026-11-16), dropping the meta-models/ path segment from divisor-guard's model in the session_metadata copy — while the authoritative top-level plan_entries carried the full slug — made consolidate_dispatch fail closed with "run ... model provenance does not match its plan entry" and an INCONCLUSIVE verdict even though all six reviewers had succeeded. Recovery did not require re-running any reviewer: the persisted session-metadata.json inside the dispatch store directory (/tmp/opencode/dispatch-<correlation_id>/) was corrected to the authoritative plan value — the typo'd string occurred exactly once, so a targeted replace was safe after asserting the count — and consolidate_dispatch was re-called with the same correlation_id and provenance. Also: diff_path is a required dispatch_review_runs parameter for non-triage modes; omitting it fails input validation before any run starts.
