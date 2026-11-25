---
tag: documentation-pipeline
author: jay-flowers
category: pattern
created_at: 2026-11-07T20:23:27Z
identity: documentation-pipeline-20261107T202327-jay-flowers
tier: draft
---

Dewey website docs-sync remediation pattern (established by PR #157, search-compound-terms): when a review flags a missing unbound-force/website docs-sync issue for a user-facing change, complete remediation has three parts — (1) create the issue in unbound-force/website with label `docs` and title `docs: sync dewey <feature> documentation`, naming the changed behavior and the affected `content/docs/` pages (exemplar: website#390); (2) reference the issue number in the OpenSpec proposal.md Impact section; (
