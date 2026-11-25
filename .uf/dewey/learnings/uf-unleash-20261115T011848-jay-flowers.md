---
tag: uf-unleash
author: jay-flowers
category: gotcha
created_at: 2026-11-15T01:18:48Z
identity: uf-unleash-20261115T011848-jay-flowers
tier: draft
---

Two child-session dispatch environment failures can block /uf.unleash mid-pipeline (observed 2026-11-14 on quick/require-code-owner-review-for-compiled-k). First, uf_delegate_command prompt-only delegations can fail repeatedly with "Cannot connect to API" against https://models.corp.redhat.com when the model-matrix resolution chain selects an unreachable gateway for the child session; passing an explicit `model` parameter (the working host model, e.g. enmaas/rits/zai-org/glm-5-3) wins the precedence chain outright, skips matrix resolution, and unblocks the delegation — the identical call succeeded immediately with the override after two failures without it. Second, the gaze-reporter agent used by review-council Phase 1b cannot be invoked from a /uf.unleash session because the subagent depth limit is 1; proceed with the gaze CLI threshold data already captured by the pre-flight gate and record the Phase 1b agent skip as informational.
