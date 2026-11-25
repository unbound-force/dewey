---
tag: documentation-pipeline
author: jay-flowers
category: pattern
created_at: 2026-11-07T20:14:55Z
identity: documentation-pipeline-20261107T201455-jay-flowers
tier: draft
---

Dewey website-sync issues (unbound-force/website, docs label) are cross-repo tracking artifacts whose operative "What needs updating" section defers to the canonical README row in the dewey repo. When a later feedback-fix commit rewords shared documentation prose (for example, re-framing a tool-consumer enumeration as illustrative and adding journal_range), the sync issue's background "What changed" prose is deliberately left stale: the deferral keeps the issue operative because the website writer syncs from the current README, not the issue background. Curator reviewers should verify the issue's operative section points at the current canonical doc instead of re-flagging the stale background prose each iteration. Verified in review-council iteration 2 of read-tools-bare-names (dewey PR #163, unbound-force/website#391).
