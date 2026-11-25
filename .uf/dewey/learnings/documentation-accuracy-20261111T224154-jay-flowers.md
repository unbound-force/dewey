---
tag: documentation-accuracy
author: jay-flowers
category: gotcha
created_at: 2026-11-11T22:41:54Z
identity: documentation-accuracy-20261111T224154-jay-flowers
tier: draft
---

Dewey's "51 MCP tools" documentation claim counts all distinct tool registrations in server.go across configurations (50 AddTool call sites on main plus verify_candidates in PR #197), while server_test.go's TestNewServer_ToolCount wantMax bounds per-configuration maxima — the vault-only `reload` tool and the seven Logseq-only DataScript tools (get_references, query_datalog, flashcards×3, whiteboards×2) never coexist in one backend, so the documented total legitimately exceeds the test's wantMax by one. Reviewers verifying tool-count claims must compare against the total AddTool call-site count in server.go, not the per-configuration test bounds.
