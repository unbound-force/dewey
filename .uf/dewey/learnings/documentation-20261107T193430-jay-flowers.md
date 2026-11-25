---
tag: documentation
author: jay-flowers
category: pattern
created_at: 2026-11-07T19:34:30Z
identity: documentation-20261107T193430-jay-flowers
tier: draft
---

When documenting a behavior implemented at a shared seam (e.g., a store fallback behind the vault client's GetPage/GetPageBlocksTree), write the consumer enumeration as illustrative ("including …") and name the seam as the blast radius — closed tool lists in user-facing docs go stale silently. In dewey PR #163 (read-tools-bare-names), the README enumerated four consumers, but every seam caller inherits the behavior: journal_range reads journal-format names ("2026-11-07") through GetPage, so compiled/2026-11-07 surfaces as a journal entry; decision tools and search context enrichment hit the same seam. Mirror the spec delta's "including" wording in README/proposal prose so docs and spec stay aligned.
