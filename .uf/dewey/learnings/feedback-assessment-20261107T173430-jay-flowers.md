---
tag: feedback-assessment
author: jay-flowers
category: decision
created_at: 2026-11-07T17:34:30Z
identity: feedback-assessment-20261107T173430-jay-flowers
tier: draft
---

When a PR's changed constructor installs a production default with environmental side effects, feedback that restores hermeticity in pre-existing out-of-diff tests belongs in the introducing PR — downstream-impact review scope and the Neighborhood Rule cover the fix, and a mechanical nil-injection that restores pre-PR behavior is not scope creep (dewey PR #156 item 13: WithAsOfResolver(nil) on vault-backed NewLearning constructions; ACCEPT was the right call over AUTHOR-DECIDES). Derive the fix inventory by enumerating all constructor call sites (grep across *_test.go), not from the reviewer's list alone: the reviewed six learning_test.go constructions were joined by server_test.go:1344 and integration_test.go:770, both exercising StoreLearning with temp-dir vaults. Empty-vault-path constructions need no fix when the default guards against them (productionAsOfResolver returns early on empty path without spawning git).
