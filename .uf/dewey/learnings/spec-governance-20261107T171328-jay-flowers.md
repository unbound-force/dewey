---
tag: spec-governance
author: jay-flowers
category: pattern
created_at: 2026-11-07T17:13:28Z
identity: spec-governance-20261107T171328-jay-flowers
tier: draft
---

Deliberate deferrals documented in code comments ("known gap — tracked by the follow-up issue") go stale the moment the follow-up lands unless comment removal is a tracked task. In dewey, spec 017's FR-017 preserved routing Vertex embedding doctor checks through Ollama with an in-code comment pointing at the follow-up issue (filed by spec 017 task T047 as dewey #143). The closing change (doctor-vertex-embedding, PR #158) kept the traceability chain intact by making "drop the FR-017 shared-path comment" an explicit tasks.md item (1.1), recording originating_issue: 143 in .openspec.yaml, and citing FR-017 in the proposal's Why — so superseding a prior spec's deliberate deferral is itself spec-governed rather than an untracked behavior change.
