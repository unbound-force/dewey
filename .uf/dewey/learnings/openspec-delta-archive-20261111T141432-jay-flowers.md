---
tag: openspec-delta-archive
author: jay-flowers
category: pattern
created_at: 2026-11-11T14:14:32Z
identity: openspec-delta-archive-20261111T141432-jay-flowers
tier: draft
---

When an OpenSpec delta must extend a requirement owned by a still-unarchived change, using ADDED with "amend by reference" prose works at authoring time because MODIFIED would be refused (no target spec in openspec/specs/). At archive time both requirements land in the same main spec, so an exclusive enumeration in the owner ("exactly these six tools") and an additive one in the delta ("additionally exactly one tool") become contradictory. Convert the delta to MODIFIED once the owning change archives, or have the owner's text say the enumeration is extensible.
