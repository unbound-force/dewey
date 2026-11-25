---
tag: search-index
author: jay-flowers
category: gotcha
created_at: 2026-11-07T18:54:51Z
identity: search-index-20261107T185451-jay-flowers
tier: draft
---

Dewey's vault search index measures token and component lengths in bytes: tokenize's len(w) >= 2 filter (search_index.go:265) and splitCompound's len(p) >= 2 filter (search_index.go:282) both use Go's len() on strings, which counts bytes, not runes. Spec and GoDoc prose that says "at least 2 characters" implies rune semantics (utf8.RuneCountInString); the two diverge for multi-byte scripts — a single CJK rune (3 bytes) or 2-byte accented Latin character passes a >=2 byte threshold, so compound x-中 expands to the component 中 and matches every block containing it. When a spec MUST states a character-count threshold but the implementation uses len(), prefer aligning the wording to bytes ("at least 2 bytes, matching tokenize's minimum term size"): it is the lower-risk fix and preserves tokenize's inherited behavior (Neighborhood Rule — the 37 graphthulhu MCP tools must keep identical results). Switching to utf8.RuneCountInString changes pre-existing indexing behavior for multi-byte content and should be a deliberate, separately-spec'd decision.
