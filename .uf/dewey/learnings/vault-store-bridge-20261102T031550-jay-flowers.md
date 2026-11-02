---
tag: vault-store-bridge
author: jay-flowers
category: gotcha
created_at: 2026-11-02T03:15:50Z
identity: vault-store-bridge-20261102T031550-jay-flowers
tier: draft
---

Case-insensitive lookup parity between an in-memory Go index and a SQLite tier requires checking the collation's alphabet, not just its case-folding behavior. In dewey (PR #138), the in-memory vault tier keys pages with strings.ToLower (Unicode-aware) while the store fallback uses `WHERE name = ? COLLATE NOCASE` (ASCII-only). The recorded driver was ASCII (the uppercase "T" in learning-page ISO timestamps, where input-lowercasing would break exact-name lookups), so SQL-side NOCASE was correct — but for store-only pages with non-ASCII cased names, a wrong-case lookup misses in the live fallback yet succeeds after a restart, because startup hydration re-keys with ToLower. When porting case-insensitive semantics across language/SQL boundaries, verify both folders use the same alphabet and either document the divergence (GoDoc + spec scoping) or fold in Go where the semantics are already correct.
