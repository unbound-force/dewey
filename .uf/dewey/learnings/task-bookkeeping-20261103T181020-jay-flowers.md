---
tag: task-bookkeeping
author: jay-flowers
category: gotcha
created_at: 2026-11-03T18:10:20Z
identity: task-bookkeeping-20261103T181020-jay-flowers
tier: draft
---

A tasks.md test task that enumerates specific contract scopes (dewey spec 017 T011: "exponential growth with jitter bounds ... prompt context cancellation during a wait") is complete only when each enumerated scope has an asserting test — a green package is not completion. A suite passes with no-op wait stubs, discarded delay values, and a cancellation test that cancels before the first request, leaving enumerated scopes undelivered while the checkbox reads [x]. Because task-completion checkboxes are gatekeeping values, a [x] that outruns its enumerated scopes is a governance finding independent of the coverage gap itself. Review technique: when a test task is claimed complete, diff the task's enumerated scopes against the actual assertions before trusting the checkbox. Fix pattern that restores both coverage and honest bookkeeping: bound-assert per-attempt delay ranges at the injected wait seam, unit-test the real wait function directly, and cancel from inside the stub handler after the first rate-limited response with the real wait wired in.
