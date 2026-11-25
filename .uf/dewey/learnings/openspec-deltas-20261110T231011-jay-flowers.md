---
tag: openspec-deltas
author: jay-flowers
category: gotcha
created_at: 2026-11-10T23:10:11Z
identity: openspec-deltas-20261110T231011-jay-flowers
tier: draft
---

An OpenSpec delta that allocates capability-prefixed FR ids (e.g. KD-FR-001..003) against a capability whose main spec is still unarchived can collide with the pending sibling change's later archive: the sibling delta may number its requirements in the same range, or archiving this change first creates a main spec with only the new requirements and mixed id styles. Number the older capability's requirements first, or postpone id allocation until its main spec exists.
