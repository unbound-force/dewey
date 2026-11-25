---
tag: content-pipeline
author: jay-flowers
category: gotcha
created_at: 2026-11-07T20:13:48Z
identity: content-pipeline-20261107T201348-jay-flowers
tier: draft
---

When a website docs-sync tracking issue is filed mid-review-loop (e.g., as a task 2.3 deliverable) and the source documentation is subsequently reworded to address review advisories, the tracking issue body must be refreshed in the same change. Otherwise the sync issue — the artifact the website docs will be written from — propagates the pre-fix framing to public docs, recreating the exact imprecision the advisory round corrected. Verified instance: unbound-force/website#391 (read-tools-bare-names sync) still enumerated four bare-name consumers as exhaustive after the dewey README marked the list illustrative and added journal_range.
