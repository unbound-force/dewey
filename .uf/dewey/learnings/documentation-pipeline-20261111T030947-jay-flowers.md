---
tag: documentation-pipeline
author: jay-flowers
category: gotcha
created_at: 2026-11-11T03:09:47Z
identity: documentation-pipeline-20261111T030947-jay-flowers
tier: draft
---

When a dewey change satisfies the website docs-sync gate by extending an existing issue with a scoped comment instead of filing a new one (the website#380 pattern used by dewey #166 serialize-supersession-writes), the reviewing Curator's read-only verification must run `gh issue view <N> --repo unbound-force/website --comments`: the sync record lives in the comment thread, not the issue body, so the body-only check from the PR #158 verification learning (documentation-pipeline-20261107T171326) would falsely report the gate as unsatisfied. tasks.md claims citing comment IDs (e.g. issuecomment-6784758201) are agent-authored and verify only against the live thread.
