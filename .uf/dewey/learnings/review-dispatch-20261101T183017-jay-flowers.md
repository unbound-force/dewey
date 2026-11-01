---
tag: review-dispatch
author: jay-flowers
category: gotcha
created_at: 2026-11-01T18:30:17Z
identity: review-dispatch-20261101T183017-jay-flowers
tier: draft
---

Manifest-delivery review retrieval gotcha (RD-FR-032): when a read_diff_chunk response exceeds the inline output limit, the full output is persisted to a tool-output file whose "content" field is a single escaped JSON line — the Read tool truncates that line at 2000 chars, and the Task tool may be unavailable (subagent depth limit 1 in dispatched reviewer sessions). Working fallback: grep the patch file itself for "^diff --git a/<file>" to get the section's starting line number, then Read the patch file with offset/limit to page through that file's diff section directly. Verified on review-diff-f741327f (dewey mcp-internal-store-access review): vault/vault_test.go's 1412-line section started at patch line 4114 and was reviewable in two Read calls.
