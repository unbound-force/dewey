---
tag: documentation-pipeline
author: jay-flowers
category: pattern
created_at: 2026-11-07T18:35:35Z
identity: documentation-pipeline-20261107T183535-jay-flowers
tier: draft
---

Cross-repo remediation artifacts cited inside spec artifacts are claims, not evidence. When a re-review dispatch verifies a finding like "website docs-sync issue missing," do not confirm remediation from the proposal.md/tasks.md reference alone (e.g., "unbound-force/website#390") — verify the issue directly with `gh issue view <number> --repo <owner>/<repo>`, confirming it exists, is open, carries the expected label (docs), follows the title convention, and scopes the affected pages. In PR dewey#157 (search
