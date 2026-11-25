---
tag: spec-review
author: jay-flowers
category: gotcha
created_at: 2026-11-10T23:46:51Z
identity: spec-review-20261110T234651-jay-flowers
tier: draft
---

When an OpenSpec/design artifact specifies a compare-and-swap guard on a JSON-embedded key, the predicate and the loser-disambiguation contract can disagree on the key's value domain. A predicate like COALESCE(json_extract(properties,'$.key'),'') = '' treats every non-empty JSON value (number, boolean, object, array) as claimed, but a disambiguation step that reports "the recorded value" only has a rendering for strings — so the requirement "report a warning naming the existing value" is unimplementable for wrong-type values, and a naive unchecked type assertion panics the tool handler. Rule: for every guarded key, the spec must enumerate the empty/absent shapes AND the wrong-type shapes, state that only a non-empty string is a reportable recorded value, route wrong-type values to the fail-closed hard-error branch already used for undecodable properties, and require comma-ok extraction (never an unchecked assertion). Add a table row per wrong-type value to the test matrix, because the empty/absent JSON-null/{}/malformed rows a spec usually carries do not exercise this path.
