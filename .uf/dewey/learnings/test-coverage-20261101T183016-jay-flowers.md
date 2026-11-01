---
tag: test-coverage
author: jay-flowers
category: pattern
created_at: 2026-11-01T18:30:16Z
identity: test-coverage-20261101T183016-jay-flowers
tier: draft
---

Spec-traceability trap in review and test planning: when a requirement enumerates a list of behaviors in one sentence (e.g., "enumeration filters (namespace, property, tag) MUST apply to the merged set"), each enumerated item needs its own mapped test. Partial coverage of enumerated MUST items is easy to miss because sibling-item tests (namespace, tag) create the impression the requirement is covered, and review feedback that names only a subset can silently narrow the implemented coverage below the spec's own list. In the mcp-internal-store-access change (dewey), the hasProperty-over-merged-set filter from the Unified Page Enumeration requirement ended up untested while its two siblings got dedicated integration helpers in server_test.go — caught only by diffing the spec's enumerated list against the test names. Countermeasure: when reviewing a change against its delta spec, extract every comma/parenthesis-delimited enumeration inside MUST clauses into a checklist and map each item to a concrete test before approving coverage.
