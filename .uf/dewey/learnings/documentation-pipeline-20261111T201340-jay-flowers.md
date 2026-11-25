---
tag: documentation-pipeline
author: jay-flowers
category: pattern
created_at: 2026-11-11T20:13:40Z
identity: documentation-pipeline-20261111T201340-jay-flowers
tier: draft
---

Dewey's Website Documentation Sync pattern for follow-on changes: when a change alters behavior already tracked by an OPEN issue in unbound-force/website (e.g., the docs-sync/tutorial/blog trio website#371/#376/#377), extend the open issue with a dated addendum comment instead of filing a duplicate, and record the outcome — with comment permalinks — in the change's tasks.md so reviewers can verify each addendum exists and covers the delta (gh issue view N --repo unbound-force/website --json comments). PR #195 (curator-tool-allowlist) used this pattern; verification confirmed all three #192 addenda present on open issues while `gh issue list --label docs --search curator --state open` showed no duplicate.
