---
tag: spec-accuracy
author: jay-flowers
category: gotcha
created_at: 2026-11-07T18:54:56Z
identity: spec-accuracy-20261107T185456-jay-flowers
tier: draft
---

Spec requirement prose that states a length threshold must name the unit. In Go, len() counts bytes, but "at least 2 characters" in a requirements document reads as runes to any implementer or reviewer — and for multi-byte content the two readings disagree (a single CJK rune is 3 bytes but 1 character). search-compound-terms (dewey PR #157) shipped splitCompound with len(p) >= 2 while its delta spec, proposal, design, GoDoc, and AGENTS.md entry all said "characters"; the mismatch surfaced only at review. Write "at least 2 bytes" (or "at least 2 runes, counted via utf8.RuneCountInString") and, when a new threshold deliberately reuses a pre-existing convention (tokenize's byte-length filter), say so in the spec — e.g., "matching tokenize's term-length convention" — so the archived contract matches the shipped behavior and a future implementer does not "fix" the code to match the prose (which would break Neighborhood-Rule parity for inherited tools).
