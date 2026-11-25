---
tag: spec-accuracy
author: jay-flowers
category: gotcha
created_at: 2026-11-03T20:31:36Z
identity: spec-accuracy-20261103T203136-jay-flowers
tier: draft
---

When a feedback fix replaces a security mechanism (stdlib-default redirects → explicit CheckRedirect), the artifact-alignment sweep must cover tasks.md, not just contract/plan/research. In dewey 017, wire contract/plan D7/research R8 were corrected but T007 still read "relies on net/http default cross-host redirect behavior" and T007/T013/R8 cited FR-018 for the redirect rule that lives in the spec's edge cases. Sweep pattern: after changing a mechanism, grep the entire feature directory for the superseded mechanism's name and the FR numbers cited alongside it.
