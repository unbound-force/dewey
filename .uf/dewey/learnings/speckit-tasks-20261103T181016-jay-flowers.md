---
tag: speckit-tasks
author: jay-flowers
category: gotcha
created_at: 2026-11-03T18:10:16Z
identity: speckit-tasks-20261103T181016-jay-flowers
tier: draft
---

Speckit tasks.md test tasks enumerate explicit case lists (dewey spec 017 T011: "exponential growth with jitter bounds... prompt context cancellation during a wait"); the existence of a matching-named test file is not task completion. When marking a test task [x], diff the task's declared case list against the test names and assertions actually present — a partial test file passes the phase checkpoint while named contract clauses stay unverified. Observed in dewey PR #144: openaicompat/retry_test.go shipped three parseRetryAfter tests, so T011 was checked complete, while its exponential-bounds and wait-cancellation cases were absent and the exported Wait had zero coverage because every test replaced the waitFn seam.
