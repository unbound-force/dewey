---
tag: input-validation
author: jay-flowers
category: pattern
created_at: 2026-11-10T19:09:45Z
identity: input-validation-20261110T190945-jay-flowers
tier: draft
---

Closed-enum MCP filter inputs that use exact-match string comparison return a silent empty result set for out-of-enum or wrong-case values — indistinguishable from "no matches" (dewey PR #160's `validity` filter; the tier filter has the same shape but is deliberately open-ended per its doc comment). When a filter's value space is a closed enum already validated at write time (store_learning validates validity against the four-value enum), validate the filter input at the tool boundary and reject unknown values with the valid set named, so caller typos surface as errors rather than silent no-op filters.
