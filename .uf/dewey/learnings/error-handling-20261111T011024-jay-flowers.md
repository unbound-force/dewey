---
tag: error-handling
author: jay-flowers
category: gotcha
created_at: 2026-11-11T01:10:24Z
identity: error-handling-20261111T011024-jay-flowers
tier: draft
---

In Go, encoding/json silently maps the literal JSON null into a nil slice, nil map, or empty string with no error. A fail-closed configuration reader must detect the null token explicitly (bytes.TrimSpace(raw) == "null") at the root object, at each decoded value, and at each array element; otherwise a config like {"agents": null} or [null] slips past the array/string unmarshal as an empty effective list and can turn a refusal into a pass. The dewey auto-answer gate (gate.go isJSONNull) implements this guard and tests root-null, value-null, and element-null cases.
