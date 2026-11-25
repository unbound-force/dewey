---
tag: verify-candidates
author: jay-flowers
category: gotcha
created_at: 2026-11-11T22:41:48Z
identity: verify-candidates-20261111T224148-jay-flowers
tier: draft
---

When a SQLite store query admits page names case-insensitively (COLLATE NOCASE range predicates, like dewey's ListDraftCandidatePages over learning/compiled/knowledge), every downstream Go string operation deriving values from those names must be case-insensitive too. In dewey PR #197's verify_candidates, learningIdentityOf guarded with strings.EqualFold (via hasNamespacePrefix) but stripped the prefix with case-sensitive strings.TrimPrefix, so a page stored as "Learning/Caps" was admitted to the queue and dispatched as a learning, yet its Identity field reported the full page name instead of the bare identity; pageMatchesTag's name-prefix form (strings.HasPrefix) has the same asymmetry and silently misses case-variant names on tags-scoped calls. Fix pattern: when the dispatch comparison is EqualFold, perform the strip with a slice (name[len(prefix):]) after the fold check succeeds, never strings.TrimPrefix.
