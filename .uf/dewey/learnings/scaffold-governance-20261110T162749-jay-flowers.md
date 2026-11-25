---
tag: scaffold-governance
author: jay-flowers
category: pattern
created_at: 2026-11-10T16:27:49Z
identity: scaffold-governance-20261110T162749-jay-flowers
tier: draft
---

When a scaffold refresh must carry a local security fix on a tool-owned file (dewey PR #167, commit a6b988b): declare the exception explicitly in the PR body (which file, which commit, what it changes), file the durable fix upstream (unbound-force/unbound-force#1011), and let the next refresh converge. The interim mechanism — replacing shell heredocs that write untrusted text with host file-write-tool instructions, keeping only fully-quoted gh invocations in shell — eliminates content-based delimiter termination (a quoted heredoc still ends at any line exactly matching its published delimiter) while preserving the approval gate, which should be extended to display the content it now covers (issue titles/bodies) before posting.
