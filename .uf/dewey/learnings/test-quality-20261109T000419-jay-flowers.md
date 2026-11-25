---
tag: test-quality
author: jay-flowers
category: gotcha
created_at: 2026-11-09T00:04:19Z
identity: test-quality-20261109T000419-jay-flowers
tier: draft
---

Dual-write fix paths (persistent store + markdown backup) that write the store first leave a partial mutation when the second write fails. A test that asserts only the returned count or the error value passes while the store row was already mutated, and the count misrepresents the actual state to callers. Always assert the post-failure state of every written sink (store row and backup file), not just the error or the "fixed" count.
