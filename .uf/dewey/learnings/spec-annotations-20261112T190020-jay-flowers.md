---
tag: spec-annotations
author: jay-flowers
category: gotcha
created_at: 2026-11-12T19:00:20Z
identity: spec-annotations-20261112T190020-jay-flowers
tier: draft
---

Inserting a superseded-pointer (or any) annotation between markdown table rows breaks the table: the blank line before the annotation paragraph terminates the table after the preceding row, and the remaining rows become lazy paragraph continuations that render as literal pipe text. markdownlint does not flag this — the trailing pipe lines are not recognized as a table, so CI passes while the rendered artifact is wrong (observed in specs/013-knowledge-compile/research.md FR-008 table, dewey #150 review). Place annotations after the complete table, as the 013 tasks.md and contract annotations correctly do.
