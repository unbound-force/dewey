---
tag: go-strings
author: jay-flowers
category: gotcha
created_at: 2026-11-07T18:54:53Z
identity: go-strings-20261107T185453-jay-flowers
tier: draft
---

Go's len() on strings measures bytes, not characters. When spec or GoDoc wording phrases a length threshold as "N characters" while the implementation uses len(), single multi-byte runes slip through the filter: a 1-character CJK rune is 3 bytes and passes a >=2-bytes check (dewey PR #157: splitCompound's `len(p) >= 2` vs the delta spec's "components of at least 2 characters" — `x-中` expands pass 2 to every block containing 中). Observed in dewey search-compound-terms review. Prefer aligning the spec/GoDoc wording to the implemented byte measure when the byte convention is pre-existing and shared across the component (dewey's tokenize and splitCompound both use len >= 2; switching tokenize to utf8.RuneCountInString would drop single-CJK-character terms from the index and change inherited search behavior — Neighborhood Rule violation). Reserve rune-count semantics for new code that genuinely means characters, and write "bytes" or "runes" explicitly in requirements so RFC 2119 MUST wording carries its exact meaning.
