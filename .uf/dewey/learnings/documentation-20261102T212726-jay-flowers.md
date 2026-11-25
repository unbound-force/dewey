---
tag: documentation
author: jay-flowers
category: gotcha
created_at: 2026-11-02T21:27:26Z
identity: documentation-20261102T212726-jay-flowers
tier: draft
---

When documenting retry budgets, distinguish the initial attempt from retries. A loop written `for attempt := 0; attempt <= MaxRetries; attempt++` makes `MaxRetries + 1` total requests (e.g. MaxRetries=5 means 6 requests). Documentation that says "up to 5 attempts" for such a loop is off by one; prefer the spec phrasing "one initial attempt plus up to N retries" and cross-check README claims against the FR text and the retry constant before merge.
