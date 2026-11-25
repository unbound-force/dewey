---
tag: spec-reconciliation
author: jay-flowers
category: pattern
created_at: 2026-11-10T23:26:59Z
identity: spec-reconciliation-20261110T232659-jay-flowers
tier: draft
---

Reconciling a shipped code state with a completed spec's invariant (dewey issue #155, T047 precedent): keep the invariant line verbatim and add an adjacent "Known divergences from invariant N" subsection. The subsection names, per divergence: the edge and the concrete symbol/files that create it (e.g. llm imports embed solely for embed.DefaultOllamaEndpoint, plus the in-package llm/provider_test.go site); the change that actually introduced it, verified from primary artifacts; the artifact that records it and when; whether the edge is live, and the full live outbound edge list; and the direct-vs-transitive scope when a MUST NOT names a package that is still reached indirectly (llm does not import openaicompat directly but reaches it through embed). Cite OpenSpec changes by name and scenario rather than by mutable path, because active changes move under openspec/changes/archive/<date>-<name>/ and a path citation goes stale. External `_test` packages (package foo_test) do not count toward a package's leaf property; in-package test files (package foo) do.
