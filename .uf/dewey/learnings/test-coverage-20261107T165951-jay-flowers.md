---
tag: test-coverage
author: jay-flowers
category: gotcha
created_at: 2026-11-07T16:59:51Z
identity: test-coverage-20261107T165951-jay-flowers
tier: draft
---

Filter-rule boundary tests need the mixed case, not just all-qualify and none-qualify. When a predicate filter's output feeds downstream behavior, the partial-qualify case is where behavior qualitatively shifts: dewey's splitCompound maps 'x-kb' to ['kb'], silently dropping the 1-char part along with its AND-query constraint, so the compound query 'x-kb' matches any block containing 'kb' — broader than the whole-term fallback documented for the none-qualify case ('a-b'). Enumerate all partitions of the boundary (all/none/some parts qualify) whenever a filter's output feeds a second behavior. Found in dewey PR #157 via divisor-testing feedback on the TestSplitCompound table.
