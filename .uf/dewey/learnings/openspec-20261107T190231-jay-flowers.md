---
tag: openspec
author: jay-flowers
category: gotcha
created_at: 2026-11-07T19:02:31Z
identity: openspec-20261107T190231-jay-flowers
tier: draft
---

Dewey's OpenSpec corpus diverges from the meta-prompt convention pack on requirement numbering: MP-002/MP-013/MP-014 (deployed 2026-11-01, commit 599ee49) require FR-NNN ids, id lines, and allocation headers in deltas, but only 2 of ~30 deltas under openspec/changes/ carry FR ids on requirements (content-sanitizer's FR-SAN-001..010, index-pipeline-performance's FR-100..102), none carry allocation headers, and all main capability specs (openspec/specs/) use bare-named '### Requirement:' headings with no numbering registry. A reviewer seeing a bare-named delta should classify the gap LOW — corpus-consistent alignment judgment, not a defect. The durable fix is repo-level (number the OpenSpec main specs and update /opsx-propose to emit ids, or scope the rules via meta-prompt-custom.md), not per-delta retrofits that make one file diverge from both extant formats.
