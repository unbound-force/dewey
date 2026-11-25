---
tag: security
author: jay-flowers
category: gotcha
created_at: 2026-11-11T02:07:53Z
identity: security-20261111T020753-jay-flowers
tier: draft
---

Go's encoding/json unmarshals a JSON null into a slice or string destination as a no-op that returns no error: json.Unmarshal([]byte("null"), &[]json.RawMessage{}) and into a string both succeed leaving the zero value. A fail-closed config parser that validates by unmarshal-error alone will silently treat `null` (root, key value, or array element) as an empty list or empty entry and pass. Explicitly compare the trimmed json.RawMessage against the literal "null" at every level (root object, target key, each array element) before unmarshaling — see isJSONNull in dewey's gate.go, added after the null-slip path was identified during PR #187 review.
