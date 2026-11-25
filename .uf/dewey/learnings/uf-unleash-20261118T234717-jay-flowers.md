---
tag: uf-unleash
author: jay-flowers
category: pattern
created_at: 2026-11-18T23:47:17Z
identity: uf-unleash-20261118T234717-jay-flowers
tier: draft
as_of: "dewey@af81010d2e226f1b1f3229c019aae4cad19004e2"
---

When a uf_delegate_command child hits its 600s wall-clock timeout but has left complete work on disk, do not re-delegate or discard: inspect the diff, run the phase gate commands directly against the tree, and mark the task complete once green (zero-waste; the run's "retryable, no retry provenance" timeout leaves no retry state). Applied twice on branch opsx/dewey-status-json-says-embeddings-are-un (2026-11-18): task 2.2 (four availability tests + helper left complete by the timed-out child; verified via go build, go vet, and the 8 embedding tests under -race) and task 3.1 (CI parity gate satisfied by the phase-2 checkpoint's execution of the exact ci.yml commands on byte-identical Go code, re-verified at the phase-3 checkpoint). Corollary: the full CI gate takes ~16 min in this repo (gaze alone exceeds 10 min and must run via nohup background + exit-code polling because the bash tool default timeout is 120s), so delegations expected to run full gates need an explicit timeout bump (uf_delegate_command timeout parameter) or orchestrator-side execution.
