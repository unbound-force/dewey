---
tag: content-pipeline
author: jay-flowers
category: gotcha
created_at: 2026-11-11T19:41:52Z
identity: content-pipeline-20261111T194152-jay-flowers
tier: draft
---

Curator review of dewey gate-config-path-parity (#191): tasks.md task 3.3 cited the durable content-pipeline pattern verbatim ("per the durable content-pipeline pattern (PR #188)") and modeled all three website legs — extend website#371 with the gate's path-parity delta, below-threshold blog determination on website#377, tutorial determination on website#376 — yet was checked complete with no posted comment id recorded anywhere in the change, even though the task text itself says "capture the posted comment id in the task output for verification". Pattern citation in task text is not execution of the pattern: the citation became ceremony while the pattern's verification mechanism (inline comment ids, verifiable via gh issue view) was skipped. Review rounds must verify the recorded ids, not the citation. This extends the omission cycle in content-pipeline-20261104T164825, content-pipeline-20261107T162516, and content-pipeline-20261111T034104 from "legs missing from the task model" to "legs modeled and claimed, verification handles absent"; the check that catches it is re-diffing the content-pipeline artifacts against the recording convention, not trusting checked boxes.
