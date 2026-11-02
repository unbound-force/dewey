---
tag: documentation
author: jay-flowers
category: gotcha
created_at: 2026-11-01T18:30:08Z
identity: documentation-20261101T183008-jay-flowers
tier: draft
---

When verifying a documentation update that enumerates affected tools (e.g., a README summary paragraph listing read tools that gained a new behavior), cross-check the enumeration against the actual changed surface in the diff (the backend methods that changed, the spec task list, or the AGENTS.md entry) rather than against the doc file's own tool tables. In mcp-internal-store-access, the README's new read-tools paragraph omitted `get_page_blocks_tree` precisely because the README never documents that tool individually — the doc author enumerated from the README's frame of reference while AGENTS.md and the website sync issue enumerated all six affected tools from the code. Enumeration-by-diff catches omissions that enumeration-by-existing-docs silently preserves.
