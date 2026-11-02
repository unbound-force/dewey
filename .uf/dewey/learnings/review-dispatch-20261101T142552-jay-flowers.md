---
tag: review-dispatch
author: jay-flowers
category: gotcha
created_at: 2026-11-01T14:25:52Z
identity: review-dispatch-20261101T142552-jay-flowers
tier: draft
---

When a design proposes new code that reads an existing data format or artifact, grep for existing consumers of that format before specifying a fresh implementation. In dewey, design D7/tasks 4.1 of mcp-internal-store-access specified best-effort reading of .uf/dewey/dewey.lock for promote's CLI lock error, but cli.go already had detectLockHolder() (used by dewey doctor) implementing the exact probe/read/graceful-degradation behavior the spec scenario required — including the stale-lock-file fallback. The design's research covered store.go's lock-writer but not cli.go's lock-reader. Reuse surveys must cover both sides of a data format's contract.
