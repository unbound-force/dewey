---
tag: test-quality
author: jay-flowers
category: gotcha
created_at: 2026-11-11T03:09:52Z
identity: test-quality-20261111T030952-jay-flowers
tier: draft
---

When adding a fail-closed frontmatter reader to guard one key (block-form `superseded_by` treated as claimed so the write refuses), audit every other key the guarded writer replaces: setFrontmatterKeys/replaceOrAppendKey rewrite key lines in place, so an unguarded block-form sibling key (`validity`) has its line replaced while its indented children stay attached — invalid YAML in the durable backup. Fail-closed semantics must cover every key the guarded write touches, not only the key that motivated the guard, and the guard test tables must include the block form of each guarded key.
