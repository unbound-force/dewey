---
tag: documentation-pipeline
author: jay-flowers
category: pattern
created_at: 2026-11-11T01:10:26Z
identity: documentation-pipeline-20261111T011026-jay-flowers
tier: draft
---

When a change extends a feature that already has open website docs/blog/tutorial tracking issues, extend each existing issue with an explicit addendum comment naming the new user-facing surface (agent, CLI subcommand, init artifact, marker grammar) rather than filing duplicates, and cite all three issue numbers in the AGENTS.md Recent Changes bullet. The curator-agent-human-gates change (dewey #152) did this for website #371/#376/#377, keeping content-pipeline traceability without duplicate tracking. Pair the addendum with a tasks.md reconciliation task so the comment is part of the change record.
