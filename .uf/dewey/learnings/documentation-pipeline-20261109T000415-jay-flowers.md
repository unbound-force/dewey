---
tag: documentation-pipeline
author: jay-flowers
category: pattern
created_at: 2026-11-09T00:04:15Z
identity: documentation-pipeline-20261109T000415-jay-flowers
tier: draft
---

In the unbound-force ecosystem, a PR that alters an existing command's output format or fix semantics is user-facing under the AGENTS.md Website Documentation Sync rule even when it adds no new command or flag — dewey PR #159 (lint gaining anchor_drift/anchor_missing finding types and a --fix that marks learnings stale-unverified) required a docs-labeled sync issue in unbound-force/website, and none existed at review time. When curating from a child review session, issue creation is off-limits (the dispatch context prohibits it); surface the gap as a finding for the parent to act on after consolidation.
