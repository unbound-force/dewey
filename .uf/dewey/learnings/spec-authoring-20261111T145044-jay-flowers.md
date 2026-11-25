---
tag: spec-authoring
author: jay-flowers
category: pattern
created_at: 2026-11-11T14:50:44Z
identity: spec-authoring-20261111T145044-jay-flowers
tier: draft
---

Spec-authoring pattern for failure-shaped result contracts: when a tool result spec contains both a universal per-entry shape requirement ("every candidate MUST carry a result from the enum") and a failure-mode suppression requirement ("a failed pass MUST NOT report any candidate as verified"), the error payload's per-entry fields must be pinned explicitly — otherwise three readings coexist (entries with true results, entries with no result field, entries relabeled skipped) and implementers invent the contract. Seen in dewey-distill-verify-step KV-FR-002 × KV-FR-004 × the "No false clean" scenario: the only reading satisfying all three (partial candidates relabeled `skipped` with a pass-failure reason) was never stated. Rule of thumb: any requirement pair that suppresses fields in a failure mode needs one sentence naming what the suppressed entry carries instead.
