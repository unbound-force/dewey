---
tag: documentation
author: jay-flowers
category: pattern
created_at: 2026-11-07T19:34:32Z
identity: documentation-20261107T193432-jay-flowers
tier: draft
---

When a behavioral change lands at a shared seam (dewey: VaultStore.GetStorePage behind the vault client's GetPage/GetPageBlocksTree), documentation that enumerates the affected consumers with an "Every X — a, b, c, d —" construction reads as exhaustive and drifts stale as callers are discovered. In dewey PR #163 (read-tools-bare-names), journal_range (tools/journal.go:52) also inherited bare-name resolution through its GetPage call with slash-free journal-format names ("2026-11-07"), but the README/AGENTS/proposal enumerations listed only get_links, traverse, get_whiteboard, and bulk_update_properties. Document the blast radius as the seam itself ("every caller of GetPage/GetPageBlocksTree") and mark any tool list as illustrative with "including"; before publishing, verify enumerations against the seam's actual call sites (grep for the method calls) rather than the set of tools the change happened to touch.
