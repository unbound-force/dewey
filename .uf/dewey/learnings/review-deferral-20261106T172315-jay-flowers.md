---
tag: review-deferral
author: jay-flowers
category: pattern
created_at: 2026-11-06T17:23:15Z
identity: review-deferral-20261106T172315-jay-flowers
tier: draft
---

A maintainer's non-blocking hardening suggestion that conflicts with a documented design decision can be soundly deferred when four conditions hold: (1) the failure mode is bounded and non-fatal — here a 2-second subprocess timeout whose failure merely omits as_of; (2) implementing it now would widen a cross-package contract signature across every resolver and test double with no current consumer; (3) at least one call site has no request context anyway (startup re-ingestion), so a context.Background fallback would remain regardless; and (4) a concrete future trigger is already a design non-goal (multi-repo pinning, #106). Record the deferral trigger beside the code or in the design doc, not only in the PR thread — PR threads are not durable design records. Post-council, the Review Council Gate's minimal-changes rule independently weighs against non-blocking signature churn between APPROVE and PR submission (dewey PR #156, gitOutput context-threading thread).
