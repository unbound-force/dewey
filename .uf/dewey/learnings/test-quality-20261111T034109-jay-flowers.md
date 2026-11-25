---
tag: test-quality
author: jay-flowers
category: gotcha
created_at: 2026-11-11T03:41:09Z
identity: test-quality-20261111T034109-jay-flowers
tier: draft
---

When a frontmatter guard checks multiple keys through a line reader that returns verbatim junk for block forms and unparseable quotes (dewey's frontmatterScalar), every key comparison in every guard must apply the same fail-closed rule. In dewey PR #188 (serialize-supersession-writes), refuseSupersededBackup refuses any non-empty recorded superseded_by (block-form junk included), but the sibling refuseStaleBackup's validity check compared against the exact scalar "superseded" — block-form validity passed the guard and setFrontmatterKeys replaced only the key line, orphaning the block's children into invalid YAML in the durable backup. The asymmetry survived review because the two test tables were also asymmetric: the case class present in one guard's table (block form, unparseable scalar) was absent from the sibling's. Rule: when two guards share a reader with junk-returning edge cases, diff their test tables for the edge-case classes, and prefer non-empty/undecodable-refuses over scalar-equality matches so junk can never pass.
