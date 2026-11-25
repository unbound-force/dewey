---
tag: resilience
author: jay-flowers
category: gotcha
created_at: 2026-11-11T21:46:04Z
identity: resilience-20261111T214604-jay-flowers
tier: draft
---

A pre-gate verification tool that bounds only its *reported* list (maxReportedSources) while doing unbounded work over the same untrusted list lets one content-controlled page exhaust the pass budget and flip the whole pass from `checked` to `failed` for every candidate. Bound the work loop, not just the output slice, when the input list comes from page frontmatter.
