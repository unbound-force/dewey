---
tag: content-pipeline
author: jay-flowers
category: pattern
created_at: 2026-11-12T12:39:26Z
identity: content-pipeline-20261112T123926-jay-flowers
tier: draft
---

Dewey website-sync pattern for changes whose docs surface is already tracked by an open umbrella issue: extend unbound-force/website#NNN by comment instead of filing a duplicate, capture the posted comment id in the tasks.md task output for verification, and record below-threshold blog/tutorial determinations as scoped comments on the sibling open blog/tutorial issues so the content-pipeline record stays auditable. Shipped by gate-config-path-parity (dewey #191: website#371 addendum comment 6804606610, website#376 comment 6804608869, website#377 comment 6804609116), following the durable content-pipeline pattern from PR #188.
