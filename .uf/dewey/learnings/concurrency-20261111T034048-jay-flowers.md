---
tag: concurrency
author: jay-flowers
category: pattern
created_at: 2026-11-11T03:40:48Z
identity: concurrency-20261111T034048-jay-flowers
tier: draft
---

CAS guard over a JSON property blob in SQLite (dewey Store.ClaimSupersession, PR #188): json_extract raises a malformed-JSON error rather than returning NULL — including on an empty-string document — so a COALESCE-only 'unclaimed' predicate is wrong twice. The working pattern: explicit `properties IS NULL OR properties = ''` disjuncts to make empty-string rows pass the guard, let malformed non-empty JSON abort the UPDATE as a fail-closed statement error, bind a row-identity predicate (source_id = constant) alongside the value predicate so a concurrently replaced row cannot be stamped, write only the guarded columns (properties + updated_at in Unix ms) rather than the whole row, and disambiguate every zero-row refusal with a re-read that hard-errors on each anomalous shape (missing page, non-learning source, non-string recorded value, undecodable properties, unclaimed-despite-guard) instead of yielding an empty winner. Comma-ok type assertions on the re-read keep a wrong-typed recorded value from surfacing as a legitimate outcome. Tests must cover the JSON5 divergence: SQLite's JSON parser accepts \\x hex escapes that Go's encoding/json rejects, so a document can pass the SQL guard yet fail the Go re-read — that path must fail closed, and only a test seeding such a document proves it.
