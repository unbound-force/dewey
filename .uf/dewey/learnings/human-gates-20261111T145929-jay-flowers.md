---
tag: human-gates
author: jay-flowers
category: pattern
created_at: 2026-11-11T14:59:29Z
identity: human-gates-20261111T145929-jay-flowers
tier: draft
---

When a workflow spec routes a human-confirmed action through a tool call at a gate (e.g., /dewey-distill applying confirmed supersessions before promote), it must specify the gate's behavior when the call fails or warns. "Call X before Y" without the failure path lets one implementation abort and another proceed — silently dropping the human's confirmation and inverting the gate's guarantee from "recorded only after confirmation" to "confirmation may not record". Pair every gated-apply requirement with a failure scenario fixing one behavior: abort, re-ask, or surface-before-proceed. Found in dewey's explicit-supersession-in-compile Distill Review Gate Integration requirement.
