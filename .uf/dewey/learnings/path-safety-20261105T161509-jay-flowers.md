---
tag: path-safety
author: jay-flowers
category: gotcha
created_at: 2026-11-05T16:15:09Z
identity: path-safety-20261105T161509-jay-flowers
tier: draft
---

In Go, filepath.Join lexically cleans ".." sequences instead of rejecting them: filepath.Join(root, ".uf/dewey", "learnings", "../../x.md") resolves to root/.uf/x.md, and enough levels escape root entirely. Using Join satisfies the "use path-joining utilities" rule (go.md SC-003) but NOT the containment rule (SC-004) — Join is a normalizer, not a sandbox. Path confinement requires an explicit check: validate the component's character class before the Join (dewey learning identities are ^[a-z0-9-]+$ because normalizeTag/resolveAuthor sanitize tool-created ones), or assert the cleaned result remains under the intended directory. Pattern from dewey PR #156 (tools/learning_temporal.go markSupersededFile): an identity stripped from a store page name flowed into a Join unchecked; the identity was forgeable because reIngestLearnings builds learning/ page names verbatim from untrusted git-synced frontmatter — any string lifted from page metadata and used as a path component inherits that metadata's trust level.
