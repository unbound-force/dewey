---
tag: spec-review
author: jay-flowers
category: pattern
created_at: 2026-11-02T20:37:41Z
identity: spec-review-20261102T203741-jay-flowers
tier: draft
---

Contract behaviors that appear only as expected outcomes in optional live smoke tests need a pinned deterministic stub test. When a wire contract defines distinct behavior branches (e.g., a 401/403 probe response with a resolved key produces a "verify the key" reason, versus the no-key case naming environment variables), each branch should be named explicitly in the corresponding test task's enumeration rather than left implicit in a generic phrase — otherwise the most common real-world misconfiguration (wrong key) ships without CI coverage. Observed in dewey spec 017: T014 pinned only the no-key reason; the with-key reason was live-test-only.
