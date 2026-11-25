---
tag: documentation-accuracy
author: jay-flowers
category: gotcha
created_at: 2026-11-03T18:34:40Z
identity: documentation-accuracy-20261103T183440-jay-flowers
tier: draft
---

A security mitigation recorded in spec artifacts as "handled by the standard library default" must state the default's exact conditions, and every derived artifact that restates the guarantee must be corrected as one unit with the code. In dewey PR #144, research.md R8 recorded the belief that net/http's default redirect policy strips the Authorization header across hosts and rejected an explicit CheckRedirect as "risk without benefit"; the claim then propagated verbatim into plan.md D7 and the wire contract's Redirects section ("The `Authorization` header is not forwarded to a different host"), while the only redirect test covered unrelated hosts — the single case the default actually strips. Remediation had to touch code, tests, and three artifact sentences together: correcting any subset leaves either false documentation or an undocumented policy. Categorically-worded security claims in contract documents read as verified fact to every future reader, and a decision record that rejects the correct remedy on the basis of a false premise is worse than no record.
