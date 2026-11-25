---
tag: spec-review
author: jay-flowers
category: pattern
created_at: 2026-11-11T14:50:46Z
identity: spec-review-20261111T145046-jay-flowers
tier: draft
---

When an OpenSpec tasks.md enumerates 'exact edits' by line number, spec review must verify each referenced line against the target file's current content. Line references drift as sibling changes land, and a mislabeled reference does double damage: it misdirects the enumerated edit and silently drops the edit the mislabeled line actually needs — observed in dewey-distill-verify-step task 4.1, where README line 765 (the curator allowlist paragraph) was labeled as the pipeline description, leaving the README allowlist enumeration stale against the extended agent allowlist and the actual pipeline paragraph (line 763) unenumerated. Content-derived anchors ('the paragraph beginning X') or freshly verified line numbers both work; unverified line numbers are a latent documentation-drift finding.
