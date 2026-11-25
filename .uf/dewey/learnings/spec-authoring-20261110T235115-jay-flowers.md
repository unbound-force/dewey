---
tag: spec-authoring
author: jay-flowers
category: gotcha
created_at: 2026-11-10T23:51:15Z
identity: spec-authoring-20261110T235115-jay-flowers
tier: draft
---

Spec scenarios that assert write destinations as hard bounds must account for user-configurable paths. Dewey's knowledge-stores.yaml `path` field defaults to `.uf/dewey/knowledge/{name}` but accepts any location, so a requirement saying persisted writes "MUST land under `.uf/dewey/`" fails under legitimate configuration with no implementation fault. Scope destination assertions to the configured destination per tool (fixed paths where fixed, configured paths where configurable), and never ship agent-body policy statements that claim the default location as a guaranteed bound.
