---
tag: test-coverage
author: jay-flowers
category: pattern
created_at: 2026-11-19T01:43:55Z
identity: test-coverage-20261119T014355-jay-flowers
tier: draft
as_of: "dewey@2a7f0147afcf07964a3cb8ce9d52b7790e1a505e"
---

When a spec delta carries explicit GIVEN/WHEN/THEN scenarios, the tasks.md test tasks should enumerate those scenarios one-for-one before implementation — this PR's tasks 4.1–4.3 listed only the happy-path stamp checks, so two IBP-FR-001 scenarios (a failed pass leaves the previous stamp; the serve-startup pass stamps) shipped without any automated test even though the spec spelled them out. A cheap pre-implementation audit is to walk each scenario heading in the delta spec and confirm a task (and later a test name) exists for it; uncovered scenarios are review findings waiting to happen.
