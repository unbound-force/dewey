---
tag: cli-observability
author: jay-flowers
category: gotcha
created_at: 2026-11-16T08:27:13Z
identity: cli-observability-20261116T082713-jay-flowers
tier: draft
as_of: "dewey@f6911abc6403998f6eaf1c2bb6d09007e516a2c6"
---

`dewey init` emits its success banner (`initialized` / `re-initialized` and `run 'dewey index'`) before the final workflow-template scaffold step. Once the shared strict write discipline made that step able to fail the run, a scaffold write failure produces success lines followed by an error and exit 1 — violating the CA-FR-005 invariant that a scaffold write failure must not report success. Order success/completion logs after every fallible step, or accept the failure and keep banners last.
