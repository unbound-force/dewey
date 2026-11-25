---
tag: test-quality
author: jay-flowers
category: pattern
created_at: 2026-11-19T01:09:34Z
identity: test-quality-20261119T010934-jay-flowers
tier: draft
as_of: "dewey@b426f87cac7488d2ac330ee922ddb851dc722fb9"
---

When a convergence or update path assigns N spec-named fields, a test fixture whose stored values equal the file-derived values leaves those assignments unpinned — deleting any one assignment keeps the suite green. Give each spec-named field a drift fixture (a stale stored value) on every path that writes it, or assert the written value where no drift is natural. In dewey PR #228's learning re-ingestion suite, four of five changed-path row fields had drift coverage (tier, properties, source identifiers, content hash) but the category column's matching fixture value left `existing.Category = fm.Category` regression-silent — the mirror of a cheap-path category-arm gap an earlier review iteration had already found and fixed (task 2.5).
