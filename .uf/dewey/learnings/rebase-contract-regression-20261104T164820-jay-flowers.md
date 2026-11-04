---
tag: rebase-contract-regression
author: jay-flowers
category: gotcha
created_at: 2026-11-04T16:48:20Z
identity: rebase-contract-regression-20261104T164820-jay-flowers
tier: draft
---

When a feature branch that rewrote a function is rebased onto a base that independently extended the same function's contract, the rebase resolution that "keeps our rewrite" can silently drop the base's additions. In dewey's opsx/dewey-distill (PR 145), the branch rewrote compile's returnPromptMode to a 3-arg version while 017 (merged into main afterward) had added an unavailable_reason field and a 4-arg reason parameter to that same function; the rebase kept the rewrite and the field vanished from compile's prompt-mode payload even though curate's equivalent kept it. Detection method that worked: diff the head against the merge base and, for every contract field present in the base's pre-image of a rewritten function, grep the head for the field name — a field in a deleted line with no corresponding added line is a dropped contract. Round-2 review notes claiming a fix ("4-arg returnPromptMode") must be verified against every call site of the same-named function, since helper names are often duplicated across sibling tools (compile vs curate both define returnPromptMode) and a fix applied to one is not a fix for the other.
