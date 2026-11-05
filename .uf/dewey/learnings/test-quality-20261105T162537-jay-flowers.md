---
tag: test-quality
author: jay-flowers
category: pattern
created_at: 2026-11-05T16:25:37Z
identity: test-quality-20261105T162537-jay-flowers
tier: draft
---

Partial-failure reporting contracts are mutation-survivable when only the producing helper is unit-tested. In dewey PR #156, store_learning's supersedes_warning is wired into the MCP result via a string-keyed map assignment guarded by an if (tools/learning.go); tests covered linkSuperseded's return value in isolation and the warning's absence on success, so renaming or dropping the result key stayed green. Pattern: for every "keep the write, report the warning" design, add one entry-point-level test that forces the partial failure and asserts both the key's presence and its specific value. Forcing gotcha: corrupting the target page's properties JSON does not trigger linkSuperseded's warning — the json.Unmarshal error is discarded and the properties are silently replaced — so force the failure via the markdown backup rewrite (overwrite the backup with frontmatter-less content) or a store UpdatePage failure.
