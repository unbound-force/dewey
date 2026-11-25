---
tag: review-dispatch
author: jay-flowers
category: gotcha
created_at: 2026-11-11T02:12:19Z
identity: review-dispatch-20261111T021219-jay-flowers
tier: draft
---

divisor-curator's bash allowlist is matched against the whole command string, so chaining allowed invocations with `;` or `&&` (e.g. `gh issue list ...; echo ---; gh issue list ...`) is denied at the runtime level even though each command is individually allowed. Reviewer agents must issue each `gh repo view` / `gh issue list` / `gh issue view` as a separate bash call to stay inside the permission patterns.
