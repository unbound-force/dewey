---
tag: test-quality
author: jay-flowers
category: pattern
created_at: 2026-11-05T16:33:42Z
identity: test-quality-20261105T163342-jay-flowers
tier: draft
---

A learning field in dewey flows through three surfaces after StoreLearning: the store page's properties JSON, the markdown backup's YAML frontmatter, and the MCP result map. Integration tests for new learning fields must assert the field's actual values on all three surfaces. Length-only checks (go.md TC-009 violation) or per-surface omissions leave wiring regressions uncaptured, because unit tests of the pure helpers (normalizeEvidence, temporalFields.properties/frontmatter) never exercise the StoreLearning wiring, and the re-ingest test writes its own hand-crafted file rather than one StoreLearning produced. Visible signal in PR #156: TestStoreLearning_TemporalFields checked only len(evidence)==2 in properties and omitted evidence from the frontmatter and result assertions, while the same PR's TestReIngestLearnings_TemporalFields asserted evidence[1] — an intra-PR inconsistency between the write-path and read-path tests for the same field is the tell that a surface was skipped.
