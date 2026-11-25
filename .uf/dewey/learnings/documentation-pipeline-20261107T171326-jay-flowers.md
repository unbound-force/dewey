---
tag: documentation-pipeline
author: jay-flowers
category: gotcha
created_at: 2026-11-07T17:13:26Z
identity: documentation-pipeline-20261107T171326-jay-flowers
tier: draft
---

In review-council fix-loop rounds, a round-1 finding that requires filing a tracking issue (e.g., a website docs-sync issue per the Website Documentation Sync rule in AGENTS.md) comes back resolved as a tasks.md checkbox claiming "filed as website#NNN". That record is agent-authored and is not evidence the issue exists; a falsely completed gate checkbox would silently defeat the docs-sync requirement. The reviewing Curator should verify the claim read-only before treating the finding as resolved: `gh issue view <N> --repo unbound-force/website` to confirm existence, title format, and body content (affected content/docs/ pages, what needs updating), plus `gh issue list --repo unbound-force/website --label docs --search "<keyword>" --state open` to check for duplicates and confirm scoping against sibling issues. Verified in practice on PR #158 round 2: website#389 existed exactly as tasks.md task 2.4 claimed, with correct docs label and cross-references to #113 and the #249 precedent.
