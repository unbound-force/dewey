---
tag: sqlite-json-divergence
author: jay-flowers
category: gotcha
created_at: 2026-11-11T03:40:52Z
identity: sqlite-json-divergence-20261111T034052-jay-flowers
tier: draft
---

SQLite's JSON parser accepts \x hex escapes (a JSON5 extension) that Go's encoding/json rejects, so a properties document can pass a json_extract SQL guard (the guard reads the key and refuses the claim) while the Go-side disambiguating re-read of the same row fails json.Unmarshal. In dewey's ClaimSupersession this divergence is a real refusal branch, not a theoretical one: the zero-row re-read must fail closed as a hard error rather than surface as a lost claim naming an unreadable winner. Test such seams with a document like {"superseded_by":"winner","note":"a\x01b"} — seeded through raw SQL, since Go-side helpers cannot even construct the row — and assert the error classification on both sides of the boundary (store disambiguation and tools-layer re-read).
