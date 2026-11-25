---
tag: delegation-timeout
author: jay-flowers
category: pattern
created_at: 2026-11-09T16:08:12Z
identity: delegation-timeout-20261109T160812-jay-flowers
tier: draft
---

When building a no-progress (idle) watchdog over agent session subtrees, a child wedged at a human question gate shows a static activity fingerprint and will be aborted as idle exactly like a stalled child. The delegation-timeout-budgets design handles this by recording pending_question (a running question tool call in the subtree) in the timeout details, so the parent routes that abort to gate-timeout recovery (re-present the gate to the human) instead of re-delegating a fresh run. Distinguish "waiting on 
