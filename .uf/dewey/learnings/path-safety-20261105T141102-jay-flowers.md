---
tag: path-safety
author: jay-flowers
category: gotcha
created_at: 2026-11-05T14:11:02Z
identity: path-safety-20261105T141102-jay-flowers
tier: draft
---

In dewey, learning page names are NOT a trusted path-safe identifier space: store_learning normalizes identities to [a-z0-9-] via normalizeTag/resolveAuthor, but reIngestLearnings builds page names verbatim from the YAML frontmatter `identity` of files in .uf/dewey/learnings/ — files that are git-synced and explicitly treated as untrusted artifacts (main.go:1083). Any code that turns a store-fetched learning page name (or its `learning/`-stripped identity) into a filesystem path must validate the identity (
