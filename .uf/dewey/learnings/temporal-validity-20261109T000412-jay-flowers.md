---
tag: temporal-validity
author: jay-flowers
category: pattern
created_at: 2026-11-09T00:04:12Z
identity: temporal-validity-20261109T000412-jay-flowers
tier: draft
---

Dewey store+backup temporal-field updates (validity, superseded_by) are written by two flows — store_learning's supersedes back-link (#146) and lint's anchor-drift fix (#147). Any new writer should extract and reuse one shared helper with one partial-failure contract: a store write that succeeds while the markdown backup update fails is a partial success reported as a warning (the supersedes_warning precedent), never a total failure — otherwise fix counts under-report and the store and backup diverge.
