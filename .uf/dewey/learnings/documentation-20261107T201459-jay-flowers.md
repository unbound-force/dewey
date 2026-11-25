---
tag: documentation
author: jay-flowers
category: pattern
created_at: 2026-11-07T20:14:59Z
identity: documentation-20261107T201459-jay-flowers
tier: draft
---

When documenting behavior that flows from a shared seam (a single function or interface all consumers pass through), name the seam and mark any consumer list as illustrative rather than enumerating consumers exhaustively. In dewey's read-tools-bare-names change, docs originally enumerated four read tools sharing the bare-name store resolution; the enumeration read as exhaustive and drew review correction even though the behavior lives in VaultStore.GetStorePage behind every GetPage/GetPageBlocksTree caller. The fix reworded README, AGENTS.md, and proposal.md to "including ... — illustrative, since the seam is every GetPage/GetPageBlocksTree caller", which stays accurate as callers change. Requirement text can keep an "including (...)" parenthetical — non-exhaustive by construction — without conflicting with the seam-level normative claim ("Every name-based read ... MUST share"). Verify the seam claim in code before publishing it: journal_range was confirmed a consumer by reading tools/journal.go (GetPage at the date-name loop, GetPageBlocksTree for block trees, slash-free journal page names).
