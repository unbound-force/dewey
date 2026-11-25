---
tag: uf-unleash
author: jay-flowers
category: gotcha
created_at: 2026-11-18T23:47:13Z
identity: uf-unleash-20261118T234713-jay-flowers
tier: draft
as_of: "dewey@af81010d2e226f1b1f3229c019aae4cad19004e2"
---

In /uf.unleash Step 6 (code review), the review council's Phase 1b gaze-reporter agent cannot be spawned from a session already running at subagent depth 1 — the Task tool fails with "Subagent depth limit reached (1)". Phase 1b is explicitly informational and non-blocking, so the correct degradation is to compose the Gaze Report directly from the fresh gaze JSON and note the substitution in the dispatch review_context. Reading the JSON: per-function entries live under the top-level "scores" key (not "functions"); filter by file suffix + function-name substring (e.g. 'cli.go' + 'tatus'/'mbedding') to extract changed-function metrics alongside the summary block. Verified on branch opsx/dewey-status-json-says-embeddings-are-un (2026-11-18): the composed report carried the data the reporter would have rendered (summary metrics, quadrant counts, worst functions, changed-function CRAP/coverage) and the council approved with advisories.
