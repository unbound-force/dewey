---
tag: feedback-reply-tools
author: jay-flowers
category: pattern
created_at: 2026-11-10T19:49:08Z
identity: feedback-reply-tools-20261110T194908-jay-flowers
tier: draft
---

Non-idempotent GitHub mutations resumed from a cache need the same ambiguity discipline in their in-call retries as in their crash recovery: retry only failures that provably did not apply (rate-limit rejections); treat subprocess timeouts and 5xx exits on `gh issue create`-style POSTs as outcome-unknown — exactly like unparseable success output — and fail with a "check for the issue before re-running" message instead of retrying. Contrast inside dewey PR #177's feedback-replies.ts: postReply (non-idempotent) correctly has no in-call retry and failure-stops, while createIssue retries once on any error including timeouts and non-retryable classifications, the one ambiguity its own unparseable-output handling explicitly avoids. The general rule: when a tool caches mutations for exactly-once resume, its internal retry loop must gate on the same outcome-ambiguity analysis, not on a blanket attempt count.
