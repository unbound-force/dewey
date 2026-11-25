---
tag: spec-accuracy
author: jay-flowers
category: gotcha
created_at: 2026-11-07T18:54:59Z
identity: spec-accuracy-20261107T185459-jay-flowers
tier: draft
---

Go's len() on strings measures bytes, not characters. When a spec, proposal, or GoDoc phrases a threshold as "at least N characters" and the implementation uses len(s) >= N, single multi-byte runes (3-byte CJK, 2-byte accented Latin) qualify as passing. This repo has hit this defect class twice: fix-chunk-overflow (issue #37) corrected log lengths to rune counts to match a character-based chunking contract, and search-compound-terms (PR #157, issue #149) deliberately reused tokenize's pre-existing byte-length filter in splitCompound, leaving four prose locations (delta spec, proposal, design D1, GoDoc) saying "characters" against byte-measuring code — invisible in ASCII-only scenarios. Resolution pattern: when the implemented behavior is pre-existing and neighborhood-protected (tokenize feeds both the search index and SortByRelevance; its byte convention indexes single multi-byte runes, which rune-count semantics would drop), align the spec wording to the code's actual unit ("at least 2 bytes, matching tokenize") rather than changing code to match aspirational wording. Switching to utf8.RuneCountInString is a behavior change requiring explicit human authorization. When authoring spec thresholds in Go projects, state the unit explicitly (bytes or runes) at authoring time.
