---
tag: spec-conventions
author: jay-flowers
category: gotcha
created_at: 2026-11-07T19:02:33Z
identity: spec-conventions-20261107T190233-jay-flowers
tier: draft
---

When a convention-pack MUST rule (e.g., FR-NNN requirement identifiers, meta-prompt MP-002/MP-013/MP-014) is absent from the majority of the existing artifact corpus, a per-PR retrofit of one artifact fragments consistency instead of establishing it — and partial compliance (ids without an MP-014 allocation header) is still noncompliance. The remediation scope is the corpus, not the artifact: route adoption through a deliberate repo-wide alignment pass or the delta-to-main-spec sync boundary, and disposition per-PR advisories as accepted-with-deferral (a recorded commitment) rather than fixed-in-place or silently ignored. Verified in dewey PR 157 (search-compound-terms): 5 of 8 deltas lack FR ids, 0 of 8 have allocation headers, openspec/specs/ main specs use bare '### Requirement:' headings; the reviewer advisory was LOW and framed as alignment judgment, not defect.
