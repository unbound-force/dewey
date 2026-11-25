---
tag: content-pipeline
author: jay-flowers
category: pattern
created_at: 2026-11-11T21:12:21Z
identity: content-pipeline-20261111T211221-jay-flowers
tier: draft
---

Dewey website-sync traceability pattern for OpenSpec changes that extend existing website umbrella issues instead of filing new ones: post the docs addendum as a comment on the open umbrella issue (unbound-force/website#371 for /dewey-distill), record below-threshold blog/tutorial determinations as scoped comments on the open sibling issues (website#376/#377) so future councils can verify the assessment, capture the posted comment IDs in tasks.md for deterministic verification — and mirror the references into the AGENTS.md Recent Changes bullet ('Website: docs website#NNN, tutorial website#NNN, blog website#NNN, each extended by comment'). The bullet is the top-level durable record; comment IDs in tasks.md alone leave the linkage invisible from AGENTS.md (gap found in dewey PR #196 / gate-config-path-parity: all three comment legs verified via gh, but the new bullet omitted the references every sibling bullet carries).
