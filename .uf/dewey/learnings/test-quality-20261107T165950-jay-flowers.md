---
tag: test-quality
author: jay-flowers
category: pattern
created_at: 2026-11-07T16:59:50Z
identity: test-quality-20261107T165950-jay-flowers
tier: draft
---

Table-driven tests for split/filter functions must include the mixed-qualifier boundary — an input where some elements pass the filter and others do not — not just all-pass and none-pass. The mixed case is where silent-drop semantics first become observable, and it is the case design prose most often leaves undocumented. In dewey PR #157 (search-compound-terms), splitCompound kept parts ≥2 chars; the test table covered only kb-review-adjud-01 (all parts qualify) and a-b (none qualify), while splitCompound("x-kb") → ["kb"] silently broadened query expansion to any block containing "kb" — caught only in review (divisor-testing, LOW).
