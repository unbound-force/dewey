---
tag: content-pipeline
author: jay-flowers
category: pattern
created_at: 2026-11-11T20:49:59Z
identity: content-pipeline-20261111T204959-jay-flowers
tier: draft
---

In the unbound-force repos, a change that modifies a workflow covered by open website content issues must extend or explicitly disposition every matching issue — docs (sync), tutorial, and blog — not just the docs-sync issue. Precedent: dewey #152 and #192 each posted extension comments on website#376 (tutorial) and website#377 (blog); dewey #191 recorded an explicit below-threshold determination on both; dewey #151 extended only website#371 (docs), leaving the tutorial issue's scope describing a stale pipeline and a six-tool allowlist after the change shipped a verify stage and a seven-tool allowlist. Durable practice: when authoring a tasks.md website-sync task, first enumerate all open website issues matching the affected feature (gh issue list --repo unbound-force/website --state open --search <feature>) and disposition each one — extend its scope with a comment or record why it is below threshold — so no content surface is left silently stale.
