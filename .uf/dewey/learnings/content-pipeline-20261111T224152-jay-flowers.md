---
tag: content-pipeline
author: jay-flowers
category: pattern
created_at: 2026-11-11T22:41:52Z
identity: content-pipeline-20261111T224152-jay-flowers
tier: draft
---

When a change inserts a step into a multi-step workflow that open website issues track across docs, tutorial, and blog streams (unbound-force/website#371/#376/#377 for /dewey-distill), extend every tracking issue — or record a per-issue determination — in the same change. Verified on dewey PR #197: only the docs issue was extended (comment posted, task checked), while the tutorial issue's title still described the pipeline without the new verify step at merge time and zero comments alerted the future content writer. The repo's convention (AGENTS.md Recent Changes for curator-agent-human-gates and curator-tool-allowlist) is to extend all three sibling issues by comment for /dewey-distill-related changes.
