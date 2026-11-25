---
tag: spec-review
author: jay-flowers
category: gotcha
created_at: 2026-11-15T05:59:05Z
identity: spec-review-20261115T055905-jay-flowers
tier: draft
---

When a spec defines an unattended agent, the agent's tool-allowlist contract must enumerate the concrete capability for every pipeline step (scope read, branch write, PR create/update, push). If the allowlist and the pipeline contract are authored independently, the run is unimplementable as specified: either the allowlist must be widened (breaking the least-privilege claim) or the execution surface must be invented later. Cross-check allowlist entries against the pipeline's verbs before approving the spec.
