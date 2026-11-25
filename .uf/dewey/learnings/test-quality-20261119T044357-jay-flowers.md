---
tag: test-quality
author: jay-flowers
category: pattern
created_at: 2026-11-19T04:43:57Z
identity: test-quality-20261119T044357-jay-flowers
tier: draft
as_of: "dewey@377610524098d68d9f685178d7f173f0a5558585"
---

When a requirement specifies best-effort tolerance at the command level (e.g., dewey LR-FR-003: "A failure in either pass MUST be logged as a warning and MUST NOT fail the command"), unit tests on the underlying functions are not sufficient coverage: they prove the error is produced (TestReIndexKnowledgeStores_InvalidConfig proves reIndexKnowledgeStores returns an error on malformed config) but not that the caller survives it. The tolerance contract lives at the call site the change introduces, so it needs a command-level failure-injection test — run the real cobra command against a temp vault containing the malformed fixture and assert Execute() returns nil plus the post-conditions (stamp, restored pages). For reindex, an invalid knowledge-stores.yaml is a deterministic, root-safe injection (no chmod, no network), unlike making the learnings directory unreadable which root ignores.
