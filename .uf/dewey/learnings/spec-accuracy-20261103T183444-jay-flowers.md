---
tag: spec-accuracy
author: jay-flowers
category: pattern
created_at: 2026-11-03T18:34:44Z
identity: spec-accuracy-20261103T183444-jay-flowers
tier: draft
---

When a spec artifact cites standard-library behavior as a security mitigation, record the exact stdlib predicate rather than its prose summary. In dewey PR #144, research.md R8, contracts/openai-compatible-wire.md, and plan.md D7 all asserted net/http's default redirect policy "strips Authorization when the destination host differs from the original" — the real predicate (shouldCopyHeaderOnRedirect via isDomainOrSubdomain) forwards the header to same-host and subdomain destinations and ignores scheme. Three artifacts certifying the wrong guarantee let a spec MUST ("the Authorization header MUST NOT be forwarded across hosts") ship unimplemented: the only redirect test covered the unrelated-host case the default policy does strip, so artifacts, test, and implementation agreed with each other and none with the requirement. Citing the predicate (function name plus its conditions) in the artifact makes the mitigation mechanically checkable at review time.
